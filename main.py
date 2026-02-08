#!/usr/bin/env python3
import asyncio
import json
import os
import random
import time
import uuid
from contextlib import asynccontextmanager
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from typing import Any, Dict, List, Optional, Union

import httpx
import uvicorn
from dotenv import load_dotenv
from fastapi import Depends, FastAPI, HTTPException, Query, Request, Security
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse
from fastapi.security import APIKeyHeader
from pydantic import BaseModel

import logging

logger = logging.getLogger(__name__)

load_dotenv()

# =============================================================================
# Logging Configuration
# =============================================================================

LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
)

# =============================================================================
# Pydantic Models for Batch Operations
# =============================================================================

class AudioQuality(str, Enum):
    HI_RES_LOSSLESS = "HI_RES_LOSSLESS"
    LOSSLESS = "LOSSLESS"
    HIGH = "HIGH"
    LOW = "LOW"

class BatchTrackRequest(BaseModel):
    track_ids: List[int]
    quality: AudioQuality = AudioQuality.HI_RES_LOSSLESS

class BatchArtistRequest(BaseModel):
    artist_id: int
    quality: AudioQuality = AudioQuality.HI_RES_LOSSLESS
    include_albums: bool = True
    include_eps: bool = True
    include_singles: bool = True

class TrackResult(BaseModel):
    track_id: int
    success: bool
    data: Optional[Dict[str, Any]] = None
    error: Optional[str] = None

class BatchStats(BaseModel):
    total: int
    success: int
    failed: int

class BatchTrackResponse(BaseModel):
    version: str
    results: List[TrackResult]
    stats: BatchStats

# =============================================================================
# Job Queue Dataclass for Async Operations
# =============================================================================

@dataclass
class DownloadJob:
    job_id: str
    status: str  # "pending", "processing", "completed", "failed", "cancelled"
    track_ids: List[int]
    quality: str
    created_at: datetime
    completed_at: Optional[datetime] = None
    results: Dict[int, dict] = field(default_factory=dict)
    errors: Dict[int, str] = field(default_factory=dict)
    progress: int = 0
    total: int = 0

# In-memory job store
_jobs: Dict[str, DownloadJob] = {}
_jobs_lock = asyncio.Lock()

# =============================================================================
# Bulk Download Configuration
# =============================================================================

BULK_MAX_TRACKS_PER_REQUEST = int(os.getenv("BULK_MAX_TRACKS_PER_REQUEST", "500"))
BULK_DEFAULT_CONCURRENCY = int(os.getenv("BULK_DEFAULT_CONCURRENCY", "15"))
BULK_MAX_CONCURRENCY = int(os.getenv("BULK_MAX_CONCURRENCY", "30"))
BULK_GLOBAL_SEMAPHORE = int(os.getenv("BULK_GLOBAL_SEMAPHORE", "50"))
BULK_PER_CREDENTIAL_SEMAPHORE = int(os.getenv("BULK_PER_CREDENTIAL_SEMAPHORE", "10"))
BULK_MAX_RETRIES = int(os.getenv("BULK_MAX_RETRIES", "3"))
HTTP_MAX_CONNECTIONS = int(os.getenv("HTTP_MAX_CONNECTIONS", "600"))
HTTP_KEEPALIVE_CONNECTIONS = int(os.getenv("HTTP_KEEPALIVE_CONNECTIONS", "400"))

# Global semaphore for bulk operations
_global_semaphore: Optional[asyncio.Semaphore] = None
_credential_semaphores: Dict[str, asyncio.Semaphore] = {}

# Shared HTTP client is created in app lifespan for connection reuse
_http_client: Optional[httpx.AsyncClient] = None

# One lock per credential to avoid global contention during token refreshes
_refresh_locks: Dict[str, asyncio.Lock] = {}

# Loaded credential set from token.json; each entry will be enriched with access cache
_creds: List[dict] = []


# =============================================================================
# Auto-Authentication (fetches credentials from public Gist + device flow)
# =============================================================================

_GIST_URL = "https://api.github.com/gists/48d01f5a24b4b7b37f19443977c22cd6"


async def _fetch_gist_credentials(client: httpx.AsyncClient) -> list:
    """Fetch Tidal client credentials from public GitHub Gist."""
    resp = await client.get(_GIST_URL)
    resp.raise_for_status()
    gist_data = resp.json()
    content_str = gist_data["files"]["tidal-api-key.json"]["content"]
    keys_data = json.loads(content_str)

    creds = [("fX2JxdmntZWK0ixT", "1Nn9AfDAjxrgJFJbKNWLeAyKGVGmINuXPPLHVXAvxAg=")]
    for key_entry in keys_data["keys"]:
        if key_entry.get("valid") == "True":
            creds.append((key_entry["clientId"], key_entry["clientSecret"]))

    random.shuffle(creds)
    return creds


async def _auto_auth():
    """Auto-authenticate with Tidal when no credentials exist.

    Fetches client keys from the public Gist, initiates the OAuth device flow,
    prints a verification URL, polls for authorization, and saves the token.
    """
    global CLIENT_ID, CLIENT_SECRET, REFRESH_TOKEN, USER_ID

    client = _http_client
    if client is None:
        logger.error("HTTP client not available for auto-auth")
        return

    logger.info("No credentials found. Fetching client keys from Gist...")
    try:
        gist_creds = await _fetch_gist_credentials(client)
    except Exception as e:
        logger.error("Failed to fetch Gist credentials: %s", e)
        return

    # Try each credential to get a device code
    device_code = None
    chosen_id = None
    chosen_secret = None
    verify_url = None

    for cid, csecret in gist_creds:
        logger.info("Trying client ID: %s", cid)
        try:
            resp = await client.post(
                "https://auth.tidal.com/v1/oauth2/device_authorization",
                data={"client_id": cid, "scope": "r_usr+w_usr+w_sub"},
            )
            if resp.status_code == 200:
                data = resp.json()
                device_code = data["deviceCode"]
                verify_url = data["verificationUriComplete"]
                chosen_id = cid
                chosen_secret = csecret
                break
            else:
                logger.warning("Client %s returned %d, trying next...", cid, resp.status_code)
        except Exception as e:
            logger.warning("Client %s failed: %s, trying next...", cid, e)

    if not device_code:
        logger.error("All client credentials failed. Cannot auto-authenticate.")
        return

    logger.info("=" * 60)
    logger.info("  TIDAL AUTHENTICATION REQUIRED")
    logger.info("  Visit this URL to log in: %s", verify_url)
    logger.info("  Waiting for authorization...")
    logger.info("=" * 60)
    # Also print to stdout for console visibility
    print(f"\n{'=' * 60}", flush=True)
    print(f"  TIDAL AUTHENTICATION REQUIRED", flush=True)
    print(f"\n  Visit this URL to log in:\n", flush=True)
    print(f"  {verify_url}\n", flush=True)
    print(f"  Waiting for authorization...", flush=True)
    print(f"{'=' * 60}\n", flush=True)

    # Poll for authorization (up to 10 minutes)
    token_url = "https://auth.tidal.com/v1/oauth2/token"
    token_data = {
        "client_id": chosen_id,
        "scope": "r_usr+w_usr+w_sub",
        "device_code": device_code,
        "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
    }
    basic_auth = (chosen_id, chosen_secret)
    auth_response = None

    for _ in range(120):
        try:
            resp = await client.post(token_url, data=token_data, auth=basic_auth)
            if resp.status_code == 200:
                auth_response = resp.json()
                break
            elif resp.status_code == 400:
                body = resp.json()
                error_code = body.get("error", "")
                if error_code in ("authorization_pending", "slow_down"):
                    await asyncio.sleep(5)
                    continue
                else:
                    logger.error("Auth failed: %s", body)
                    return
            else:
                await asyncio.sleep(5)
        except Exception as e:
            logger.warning("Poll error: %s, retrying...", e)
            await asyncio.sleep(5)

    if auth_response is None:
        logger.error("Authorization timed out after 10 minutes.")
        return

    # Save to token.json
    access_token = auth_response["access_token"]
    refresh_token = auth_response["refresh_token"]
    user_id = auth_response["user"]["userId"]

    token_entry = {
        "access_token": access_token,
        "refresh_token": refresh_token,
        "userID": user_id,
        "client_ID": chosen_id,
        "client_secret": chosen_secret,
    }

    tokens = []
    if os.path.exists(TOKEN_FILE):
        try:
            with open(TOKEN_FILE, "r") as f:
                existing = json.load(f)
                tokens = existing if isinstance(existing, list) else [existing]
        except (json.JSONDecodeError, IOError):
            pass
    tokens.append(token_entry)
    with open(TOKEN_FILE, "w") as f:
        json.dump(tokens, f, indent=4)

    # Add to runtime credentials
    cred = {
        "client_id": chosen_id,
        "client_secret": chosen_secret,
        "refresh_token": refresh_token,
        "user_id": str(user_id),
        "access_token": access_token,
        "expires_at": time.time() + 3600 - 60,
    }
    _creds.append(cred)

    CLIENT_ID = chosen_id
    CLIENT_SECRET = chosen_secret
    REFRESH_TOKEN = refresh_token
    USER_ID = str(user_id)

    logger.info("Authentication successful! Credential saved to %s", TOKEN_FILE)
    print(f"\n  Authentication successful! Saved to {TOKEN_FILE}\n", flush=True)


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _http_client, _global_semaphore
    _http_client = httpx.AsyncClient(
        http2=True,
        timeout=httpx.Timeout(connect=5.0, read=30.0, write=10.0, pool=30.0),
        limits=httpx.Limits(
            max_keepalive_connections=HTTP_KEEPALIVE_CONNECTIONS,
            max_connections=HTTP_MAX_CONNECTIONS,
            keepalive_expiry=60.0,
        ),
    )
    _global_semaphore = asyncio.Semaphore(BULK_GLOBAL_SEMAPHORE)

    # Auto-authenticate if no credentials were loaded at startup
    if not _creds:
        await _auto_auth()

    try:
        yield
    finally:
        if _http_client:
            await _http_client.aclose()

API_VERSION = "2.4"

# =============================================================================
# Optional API Key Authentication
# =============================================================================

API_KEY = os.getenv("API_KEY") or None
_api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)


async def verify_api_key(api_key: Optional[str] = Security(_api_key_header)):
    """When API_KEY is set, require it on every request. When unset, allow all."""
    if API_KEY is None:
        return None
    if api_key != API_KEY:
        raise HTTPException(status_code=401, detail="Invalid or missing API key")
    return api_key


app = FastAPI(
    title="HiFi-RestAPI",
    version=API_VERSION,
    description="Tidal Music Proxy",
    lifespan=lifespan,
    dependencies=[Depends(verify_api_key)],
)

# CORS: configurable via CORS_ORIGINS env var (comma-separated).
# Defaults to "*" (open) for backwards compatibility.
_cors_origins_raw = os.getenv("CORS_ORIGINS", "*")
_cors_origins = [o.strip() for o in _cors_origins_raw.split(",") if o.strip()]
app.add_middleware(
    CORSMiddleware,
    allow_origins=_cors_origins,
    allow_credentials=("*" not in _cors_origins),
    allow_methods=["*"],
    allow_headers=["*"],
)


# Config
CLIENT_ID = os.getenv("CLIENT_ID", "")
CLIENT_SECRET = os.getenv("CLIENT_SECRET", "")
REFRESH_TOKEN: Optional[str] = os.getenv("REFRESH_TOKEN")
USER_ID = os.getenv("USER_ID")
TOKEN_FILE = os.getenv("TOKEN_FILE", "token.json")
COUNTRY_CODE = os.getenv("COUNTRY_CODE", "US")

if os.path.exists(TOKEN_FILE):
    with open(TOKEN_FILE, "r") as tok:
        token_data = json.load(tok)
        if isinstance(token_data, dict):
            token_data = [token_data]

        for entry in token_data:
            cred = {
                "client_id": entry.get("client_ID") or CLIENT_ID,
                "client_secret": entry.get("client_secret") or CLIENT_SECRET,
                "refresh_token": entry.get("refresh_token") or REFRESH_TOKEN,
                "user_id": entry.get("userID") or USER_ID,
                # Access tokens in file have unknown expiry; force refresh on first use
                "access_token": None,
                "expires_at": 0,
            }
            if cred["refresh_token"]:
                _creds.append(cred)

# Add env var credential if available and unique (simple check)
if REFRESH_TOKEN:
    env_cred = {
        "client_id": CLIENT_ID,
        "client_secret": CLIENT_SECRET,
        "refresh_token": REFRESH_TOKEN,
        "user_id": USER_ID,
        "access_token": None,
        "expires_at": 0,
    }
    # Avoid adding duplicate if it was already loaded from file with same refresh token
    if not any(c["refresh_token"] == REFRESH_TOKEN for c in _creds):
        _creds.append(env_cred)

if _creds:
    CLIENT_ID = _creds[0]["client_id"]
    CLIENT_SECRET = _creds[0]["client_secret"]
    REFRESH_TOKEN = _creds[0]["refresh_token"]
    logger.info("Loaded %d Tidal credential(s)", len(_creds))
else:
    logger.warning(
        "No Tidal credentials found. "
        "Auto-auth will run at startup to guide you through Tidal login."
    )


def _pick_credential() -> dict:
    if not _creds:
        raise HTTPException(status_code=500, detail="No Tidal credentials available; populate token.json")
    return random.choice(_creds)


def _lock_for_cred(cred: dict) -> asyncio.Lock:
    key = f"{cred['client_id']}:{cred['refresh_token']}"
    lock = _refresh_locks.get(key)
    if lock is None:
        lock = _refresh_locks.setdefault(key, asyncio.Lock())
    return lock


def _semaphore_for_cred(cred: dict) -> asyncio.Semaphore:
    """Get or create a per-credential semaphore for rate limiting."""
    key = cred["refresh_token"][:16] if cred.get("refresh_token") else "default"
    sem = _credential_semaphores.get(key)
    if sem is None:
        sem = _credential_semaphores.setdefault(key, asyncio.Semaphore(BULK_PER_CREDENTIAL_SEMAPHORE))
    return sem


def _get_global_semaphore() -> asyncio.Semaphore:
    """Get the global semaphore, creating a fallback if not initialized."""
    global _global_semaphore
    if _global_semaphore is None:
        _global_semaphore = asyncio.Semaphore(BULK_GLOBAL_SEMAPHORE)
    return _global_semaphore


async def get_http_client() -> httpx.AsyncClient:
    if _http_client is None:
        raise RuntimeError("HTTP client not initialized; app lifespan did not run")
    return _http_client


async def refresh_tidal_token(cred: Optional[dict] = None):
    """Refresh a token for the provided credential set."""
    cred = cred or _pick_credential()

    async with _lock_for_cred(cred):
        if cred["access_token"] and time.time() < cred["expires_at"]:
            return cred["access_token"]

        try:
            client = await get_http_client()
            res = await client.post(
                "https://auth.tidal.com/v1/oauth2/token",
                data={
                    "client_id": cred["client_id"],
                    "refresh_token": cred["refresh_token"],
                    "grant_type": "refresh_token",
                    "scope": "r_usr+w_usr+w_sub",
                },
                auth=(cred["client_id"], cred["client_secret"]),
            )
            res.raise_for_status()
            data = res.json()
            new_token = data["access_token"]
            expires_in = data.get("expires_in", 3600)

            cred["access_token"] = new_token
            cred["expires_at"] = time.time() + expires_in - 60

            return new_token
        except httpx.HTTPError as e:
            raise HTTPException(status_code=401, detail=f"Token refresh failed: {str(e)}")


async def get_tidal_token(force_refresh: bool = False):
    return await get_tidal_token_for_cred(force_refresh=force_refresh)


async def get_tidal_token_for_cred(force_refresh: bool = False, cred: Optional[dict] = None):
    """Retrieve an access token for a specific credential; pick random if not provided."""
    cred = cred or _pick_credential()

    if not force_refresh and cred["access_token"] and time.time() < cred["expires_at"]:
        return cred["access_token"], cred

    token = await refresh_tidal_token(cred)
    return token, cred


async def make_request(url: str, token: Optional[str] = None, params: Optional[dict] = None, cred: Optional[dict] = None):
    if token is None:
        token, cred = await get_tidal_token_for_cred(cred=cred)
    client = await get_http_client()
    headers = {"authorization": f"Bearer {token}"}

    try:
        resp = await client.get(url, headers=headers, params=params)

        if resp.status_code == 401:
            # Token expired, refresh and retry
            token, cred = await get_tidal_token_for_cred(force_refresh=True, cred=cred)
            headers = {"authorization": f"Bearer {token}"}
            resp = await client.get(url, headers=headers, params=params)

        resp.raise_for_status()
        return {"version": API_VERSION, "data": resp.json()}
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 404:
            raise HTTPException(status_code=404, detail="Resource not found")
        else:
            logger.error(
                "Upstream API error %s %s %s",
                e.response.status_code,
                url,
                e.response.text,
                exc_info=e,
            )
            raise HTTPException(status_code=e.response.status_code, detail="Upstream API error")
    except httpx.RequestError as e:
        if isinstance(e, httpx.TimeoutException):
            raise HTTPException(status_code=429, detail="Upstream timeout")
        raise HTTPException(status_code=503, detail="Connection error to Tidal")


async def authed_get_json(
    url: str,
    *,
    params: Optional[dict] = None,
    token: Optional[str] = None,
    cred: Optional[dict] = None,
):
    """Perform an authenticated GET, retrying once on 401. Returns payload with updated token/cred."""

    if token is None:
        token, cred = await get_tidal_token_for_cred(cred=cred)

    client = await get_http_client()
    headers = {"authorization": f"Bearer {token}"}

    try:
        resp = await client.get(url, headers=headers, params=params)

        if resp.status_code == 401:
            token, cred = await get_tidal_token_for_cred(force_refresh=True, cred=cred)
            headers["authorization"] = f"Bearer {token}"
            resp = await client.get(url, headers=headers, params=params)

        resp.raise_for_status()
        return resp.json(), token, cred
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 404:
            raise HTTPException(status_code=404, detail="Resource not found")
        if e.response.status_code == 429:
            raise HTTPException(status_code=429, detail="Upstream rate limited")
        raise HTTPException(status_code=e.response.status_code, detail="Upstream API error")
    except httpx.RequestError as e:
        if isinstance(e, httpx.TimeoutException):
            raise HTTPException(status_code=429, detail="Upstream timeout")
        raise HTTPException(status_code=503, detail="Connection error to Tidal")


# =============================================================================
# Bulk Download Helper Functions
# =============================================================================

async def resolve_track_url(
    track_id: int,
    quality: str,
    cred: Optional[dict] = None,
) -> dict:
    """Resolve a single track's streaming URL."""
    if cred is None:
        cred = _pick_credential()

    token, cred = await get_tidal_token_for_cred(cred=cred)
    client = await get_http_client()

    track_url = f"https://tidal.com/v1/tracks/{track_id}/playbackinfo"
    params = {
        "audioquality": quality,
        "playbackmode": "STREAM",
        "assetpresentation": "FULL",
    }
    headers = {"authorization": f"Bearer {token}"}

    resp = await client.get(track_url, headers=headers, params=params)

    if resp.status_code == 401:
        token, cred = await get_tidal_token_for_cred(force_refresh=True, cred=cred)
        headers["authorization"] = f"Bearer {token}"
        resp = await client.get(track_url, headers=headers, params=params)

    resp.raise_for_status()
    return resp.json()


async def fetch_track_with_retry(
    track_id: int,
    quality: str,
    cred: Optional[dict] = None,
    max_retries: int = BULK_MAX_RETRIES,
) -> Dict[str, Any]:
    """
    Fetch a track URL with exponential backoff retry and credential rotation.
    Uses multi-level semaphores for concurrency control.
    """
    if cred is None:
        cred = _pick_credential()

    last_exception: Optional[Exception] = None

    for attempt in range(max_retries + 1):
        try:
            async with _get_global_semaphore():
                async with _semaphore_for_cred(cred):
                    return await resolve_track_url(track_id, quality, cred)
        except httpx.HTTPStatusError as e:
            last_exception = e
            status = e.response.status_code

            if status == 404:
                # Track not found - don't retry
                raise
            elif status == 429:
                # Rate limited - switch credential and retry
                cred = _pick_credential()
                delay = min(0.5 * (2 ** attempt), 10.0)
                await asyncio.sleep(delay * (0.5 + random.random()))
            elif status in {500, 502, 503, 504}:
                # Server error - retry with backoff
                delay = min(0.5 * (2 ** attempt), 10.0)
                await asyncio.sleep(delay * (0.5 + random.random()))
            else:
                # Other error - don't retry
                raise
        except httpx.TimeoutException as e:
            last_exception = e
            if attempt < max_retries:
                await asyncio.sleep(0.5 * (2 ** attempt))
            else:
                raise
        except httpx.RequestError as e:
            last_exception = e
            if attempt < max_retries:
                await asyncio.sleep(0.5 * (2 ** attempt))
            else:
                raise

    if last_exception:
        raise last_exception
    raise Exception(f"Failed to fetch track {track_id} after {max_retries} retries")


async def batch_fetch_tracks(
    track_ids: List[int],
    quality: str,
    concurrency: int = BULK_DEFAULT_CONCURRENCY,
) -> List[TrackResult]:
    """
    Fetch multiple tracks in parallel with load balancing across credentials.
    """
    sem = asyncio.Semaphore(concurrency)
    results: List[TrackResult] = []

    async def fetch_one(track_id: int, cred: dict) -> TrackResult:
        async with sem:
            try:
                data = await fetch_track_with_retry(track_id, quality, cred)
                return TrackResult(track_id=track_id, success=True, data=data)
            except httpx.HTTPStatusError as e:
                return TrackResult(
                    track_id=track_id,
                    success=False,
                    error=f"HTTP {e.response.status_code}: {e.response.text[:200]}"
                )
            except Exception as e:
                return TrackResult(
                    track_id=track_id,
                    success=False,
                    error=str(e)[:200]
                )

    # Round-robin distribute tracks across credentials
    tasks = []
    for i, track_id in enumerate(track_ids):
        cred = _creds[i % len(_creds)] if _creds else _pick_credential()
        tasks.append(fetch_one(track_id, cred))

    results = await asyncio.gather(*tasks)
    return list(results)


# =============================================================================
# API Endpoints
# =============================================================================

@app.get("/")
async def index():
    return {"version": API_VERSION, "Repo": "https://github.com/uimaxbai/hifi-api"}

@app.get("/health")
async def health():
    """Health check endpoint for load balancers and monitoring."""
    return {
        "status": "ok",
        "version": API_VERSION,
        "credentials_loaded": len(_creds),
    }

@app.get("/info/")
async def get_info(id: int):
    url = f"https://api.tidal.com/v1/tracks/{id}/"
    return await make_request(url, params={"countryCode": COUNTRY_CODE})

@app.get("/track/")
async def get_track(id: int, quality: AudioQuality = AudioQuality.HI_RES_LOSSLESS):
    track_url = f"https://tidal.com/v1/tracks/{id}/playbackinfo"
    params = {
        "audioquality": quality,
        "playbackmode": "STREAM",
        "assetpresentation": "FULL",
    }
    return await make_request(track_url, params=params)


@app.get("/recommendations/")
async def get_recommendations(id: int):
    recommendations_url = f"https://tidal.com/v1/tracks/{id}/recommendations"
    params = {"limit": "20", "countryCode": COUNTRY_CODE}
    return await make_request(recommendations_url, params=params)


@app.api_route("/search/", methods=["GET"])
async def search(
    s: Union[str, None] = Query(default=None),
    a: Union[str, None] = Query(default=None),
    al: Union[str, None] = Query(default=None),
    v: Union[str, None] = Query(default=None),
    p: Union[str, None] = Query(default=None),
    limit: int = Query(default=50, ge=1, le=100),
):
    """Search endpoint supporting track/artist/album/video/playlist queries via distinct params."""
    queries = (
        (s, "https://api.tidal.com/v1/search/tracks", {
            "query": s,
            "limit": limit,
            "offset": 0,
            "countryCode": COUNTRY_CODE,
        }),
        (a, "https://api.tidal.com/v1/search/top-hits", {
            "query": a,
            "limit": limit,
            "offset": 0,
            "types": "ARTISTS,TRACKS",
            "countryCode": COUNTRY_CODE,
        }),
        (al, "https://api.tidal.com/v1/search/top-hits", {
            "query": al,
            "limit": limit,
            "offset": 0,
            "types": "ALBUMS",
            "countryCode": COUNTRY_CODE,
        }),
        (v, "https://api.tidal.com/v1/search/top-hits", {
            "query": v,
            "limit": limit,
            "offset": 0,
            "types": "VIDEOS",
            "countryCode": COUNTRY_CODE,
        }),
        (p, "https://api.tidal.com/v1/search/top-hits", {
            "query": p,
            "limit": limit,
            "offset": 0,
            "types": "PLAYLISTS",
            "countryCode": COUNTRY_CODE,
        }),
    )

    for value, url, params in queries:
        if value:
            return await make_request(url, params=params)

    raise HTTPException(status_code=400, detail="Provide one of s, a, al, v, or p")

@app.get("/album/")
async def get_album(
    id: int = Query(..., description="Album ID"),
    limit: int = Query(100, ge=1, le=500),
    offset: int = Query(0, ge=0),
):
    album_url = f"https://api.tidal.com/v1/albums/{id}"
    items_url = f"https://api.tidal.com/v1/albums/{id}/items"

    async def fetch(url: str, params: Optional[dict] = None):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    tasks = [fetch(album_url, {"countryCode": COUNTRY_CODE})]

    max_chunk = 100
    current_offset = offset
    remaining_limit = limit

    while remaining_limit > 0:
        chunk_size = min(remaining_limit, max_chunk)
        tasks.append(
            fetch(items_url, {"countryCode": COUNTRY_CODE, "limit": chunk_size, "offset": current_offset})
        )
        current_offset += chunk_size
        remaining_limit -= chunk_size

    results = await asyncio.gather(*tasks)

    album_data = results[0]
    items_pages = results[1:]

    all_items = []
    for page in items_pages:
        page_items = page.get("items", page)
        all_items.extend(page_items)

    album_data["items"] = all_items

    return {
        "version": API_VERSION,
        "data": album_data,
    }


@app.get("/mix/")
async def get_mix(
    id: str = Query(..., description="Mix ID")
):
    """Fetch items from a Tidal mix by its ID."""
    url = "https://api.tidal.com/v1/pages/mix"
    params = {
        "mixId": id,
        "countryCode": COUNTRY_CODE,
        "deviceType": "BROWSER",
    }

    data, _, _ = await authed_get_json(url, params=params)

    header = {}
    items = []

    rows = data.get("rows", [])
    for row in rows:
        modules = row.get("modules", [])
        for module in modules:
            if module.get("type") == "MIX_HEADER":
                header = module.get("mix", {})
            elif module.get("type") == "TRACK_LIST":
                paged_list = module.get("pagedList", {})
                items = paged_list.get("items", [])

    return {
        "version": API_VERSION,
        "mix": header,
        "items": [item.get("item", item) for item in items],
    }


@app.get("/playlist/")
async def get_playlist(
    id: str = Query(..., min_length=1),
    limit: int = Query(100, ge=1, le=500),
    offset: int = Query(0, ge=0),
):
    """Fetch playlist metadata plus items concurrently."""

    playlist_url = f"https://api.tidal.com/v1/playlists/{id}"
    items_url = f"https://api.tidal.com/v1/playlists/{id}/items"

    async def fetch(url: str, params: Optional[dict] = None):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    playlist_data, items_data = await asyncio.gather(
        fetch(playlist_url, {"countryCode": COUNTRY_CODE}),
        fetch(items_url, {"countryCode": COUNTRY_CODE, "limit": limit, "offset": offset}),
    )

    return {
        "version": API_VERSION,
        "playlist": playlist_data,
        "items": items_data.get("items", items_data),
    }


def _extract_uuid_from_tidal_url(href: str) -> Optional[str]:
    """Extract and reconstruct a hyphenated UUID from a Tidal resource URL."""
    parts = href.split("/") if href else []
    return "-".join(parts[4:9]) if len(parts) >= 9 else None


@app.get("/artist/similar/")
async def get_similar_artists(
    id: int = Query(..., description="Artist ID"),
    cursor: Union[int, str, None] = None
):
    """Fetch artists similar to another by its ID using V2 API."""
    url = f"https://openapi.tidal.com/v2/artists/{id}/relationships/similarArtists"
    params = {
        "page[cursor]": cursor,
        "countryCode": COUNTRY_CODE,
        "include": "similarArtists,similarArtists.profileArt"
    }

    payload, _, _ = await authed_get_json(url, params=params)
    included = payload.get("included", [])
    artists_map = {i["id"]: i for i in included if i["type"] == "artists"}
    artworks_map = {i["id"]: i for i in included if i["type"] == "artworks"}

    def resolve_artist(entry):
        aid = entry["id"]
        inc = artists_map.get(aid, {})
        attr = inc.get("attributes", {})

        pic_id = None
        if art_data := inc.get("relationships", {}).get("profileArt", {}).get("data"):
            if artwork := artworks_map.get(art_data[0].get("id")):
                if files := artwork.get("attributes", {}).get("files"):
                    pic_id = _extract_uuid_from_tidal_url(files[0].get("href"))

        return {
            **attr,
            "id": int(aid) if aid.isdigit() else aid,
            "picture": pic_id or attr.get("selectedAlbumCoverFallback"),
            "url": f"http://www.tidal.com/artist/{aid}",
            "relationType": "SIMILAR_ARTIST"
        }

    return {
        "version": API_VERSION,
        "artists": [resolve_artist(e) for e in payload.get("data", [])]
    }


@app.get("/album/similar/")
async def get_similar_albums(
    id: int = Query(..., description="Album ID"),
    cursor: Union[int, str, None] = None
):
    """Fetch albums similar to another by its ID using V2 API."""
    url = f"https://openapi.tidal.com/v2/albums/{id}/relationships/similarAlbums"
    params = {
        "page[cursor]": cursor,
        "countryCode": COUNTRY_CODE,
        "include": "similarAlbums,similarAlbums.coverArt,similarAlbums.artists"
    }

    payload, _, _ = await authed_get_json(url, params=params)
    included = payload.get("included", [])
    albums_map = {i["id"]: i for i in included if i["type"] == "albums"}
    artworks_map = {i["id"]: i for i in included if i["type"] == "artworks"}
    artists_map = {i["id"]: i for i in included if i["type"] == "artists"}

    def resolve_album(entry):
        aid = entry["id"]
        inc = albums_map.get(aid, {})
        attr = inc.get("attributes", {})

        cover_id = None
        if art_data := inc.get("relationships", {}).get("coverArt", {}).get("data"):
            if artwork := artworks_map.get(art_data[0].get("id")):
                if files := artwork.get("attributes", {}).get("files"):
                    cover_id = _extract_uuid_from_tidal_url(files[0].get("href"))

        artist_list = []
        if art_data := inc.get("relationships", {}).get("artists", {}).get("data"):
             for a_entry in art_data:
                 if a_obj := artists_map.get(a_entry["id"]):
                     a_id = a_obj["id"]
                     artist_list.append({
                         "id": int(a_id) if a_id.isdigit() else a_id,
                         "name": a_obj["attributes"]["name"]
                     })

        return {
            **attr,
            "id": int(aid) if aid.isdigit() else aid,
            "cover": cover_id,
            "artists": artist_list,
            "url": f"http://www.tidal.com/album/{aid}"
        }

    return {
        "version": API_VERSION,
        "albums": [resolve_album(e) for e in payload.get("data", [])]
    }


@app.get("/artist/")
async def get_artist(
    id: Optional[int] = Query(default=None),
    f: Optional[int] = Query(default=None),
    skip_tracks: bool = Query(default=False),
):
    """Artist detail or album+track aggregation.

    - id: basic artist metadata + cover URLs
    - f: fetch artist albums page and aggregate tracks across albums (capped concurrency)
    - skip_tracks: if true, returns only albums without aggregating tracks (when using 'f')
    """

    if id is None and f is None:
        raise HTTPException(status_code=400, detail="Provide id or f query param")

    if id is not None:
        artist_url = f"https://api.tidal.com/v1/artists/{id}"
        artist_data, _, _ = await authed_get_json(
            artist_url,
            params={"countryCode": COUNTRY_CODE},
        )

        picture = artist_data.get("picture")
        fallback = artist_data.get("selectedAlbumCoverFallback")

        if not picture and fallback:
            artist_data["picture"] = fallback
            picture = fallback

        cover = None
        if picture:
            slug = picture.replace("-", "/")
            cover = {
                "id": artist_data.get("id"),
                "name": artist_data.get("name"),
                "750": f"https://resources.tidal.com/images/{slug}/750x750.jpg",
            }

        return {"version": API_VERSION, "artist": artist_data, "cover": cover}

    # Fetch albums and singles/EPs directly in parallel
    albums_url = f"https://api.tidal.com/v1/artists/{f}/albums"
    common_params = {"countryCode": COUNTRY_CODE, "limit": 100}

    async def _fetch(url, params):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    tasks = [
        _fetch(albums_url, common_params),
        _fetch(albums_url, {**common_params, "filter": "EPSANDSINGLES"}),
    ]

    if skip_tracks:
        tasks.append(
            _fetch(
                f"https://api.tidal.com/v1/artists/{f}/toptracks",
                {"countryCode": COUNTRY_CODE, "limit": 15},
            )
        )

    results = await asyncio.gather(*tasks, return_exceptions=True)

    unique_releases = []
    seen_ids = set()

    # Process albums (first 2 results)
    for res in results[:2]:
        if isinstance(res, Exception):
            logger.error("Error fetching artist releases: %s", res)
            continue
        for item in res.get("items", []):
            if item.get("id") and item["id"] not in seen_ids:
                unique_releases.append(item)
                seen_ids.add(item["id"])

    album_ids: List[int] = [item["id"] for item in unique_releases]
    page_data = {"items": unique_releases}

    if skip_tracks:
        top_tracks = []
        if len(results) > 2:
            res = results[2]
            if isinstance(res, Exception):
                logger.error("Error fetching top tracks: %s", res)
            else:
                top_tracks = res.get("items", [])

        return {"version": API_VERSION, "albums": page_data, "tracks": top_tracks}

    if not album_ids:
        return {"version": API_VERSION, "albums": page_data, "tracks": []}

    sem = asyncio.Semaphore(6)

    async def fetch_album_tracks(album_id: int):
        async with sem:
            album_data, _, _ = await authed_get_json(
                "https://api.tidal.com/v1/pages/album",
                params={
                    "albumId": album_id,
                    "countryCode": COUNTRY_CODE,
                    "deviceType": "BROWSER",
                },
            )

            rows = album_data.get("rows", [])
            if len(rows) < 2:
                return []
            modules = rows[1].get("modules", [])
            if not modules:
                return []
            paged_list = modules[0].get("pagedList", {})
            items = paged_list.get("items", [])
            tracks = [track.get("item", track) for track in items]
            return tracks

    results = await asyncio.gather(
        *(fetch_album_tracks(album_id) for album_id in album_ids),
        return_exceptions=True,
    )

    tracks: List[dict] = []
    for res in results:
        if isinstance(res, Exception):
            continue
        tracks.extend(res)

    return {"version": API_VERSION, "albums": page_data, "tracks": tracks}


@app.get("/cover/")
async def get_cover(
    id: Optional[int] = Query(default=None),
    q: Optional[str] = Query(default=None),
):
    """Fetch album cover data for a track id or search query."""

    if id is None and q is None:
        raise HTTPException(status_code=400, detail="Provide id or q query param")

    def build_cover_entry(cover_slug: str, name: Optional[str], track_id: Optional[int]):
        slug = cover_slug.replace("-", "/")
        return {
            "id": track_id,
            "name": name,
            "1280": f"https://resources.tidal.com/images/{slug}/1280x1280.jpg",
            "640": f"https://resources.tidal.com/images/{slug}/640x640.jpg",
            "80": f"https://resources.tidal.com/images/{slug}/80x80.jpg",
        }

    if id is not None:
        track_data, _, _ = await authed_get_json(
            f"https://api.tidal.com/v1/tracks/{id}/",
            params={"countryCode": COUNTRY_CODE},
        )

        album = track_data.get("album") or {}
        cover_slug = album.get("cover")
        if not cover_slug:
            raise HTTPException(status_code=404, detail="Cover not found")

        entry = build_cover_entry(
            cover_slug,
            album.get("title") or track_data.get("title"),
            album.get("id") or id,
        )
        return {"version": API_VERSION, "covers": [entry]}

    search_data, _, _ = await authed_get_json(
        "https://api.tidal.com/v1/search/tracks",
        params={"countryCode": COUNTRY_CODE, "query": q, "limit": 10},
    )

    items = search_data.get("items", [])[:10]
    if not items:
        raise HTTPException(status_code=404, detail="Cover not found")

    covers = []
    for track in items:
        album = track.get("album") or {}
        cover_slug = album.get("cover")
        if not cover_slug:
            continue
        covers.append(
            build_cover_entry(
                cover_slug,
                track.get("title"),
                track.get("id"),
            )
        )

    if not covers:
        raise HTTPException(status_code=404, detail="Cover not found")

    return {"version": API_VERSION, "covers": covers}


@app.get("/lyrics/")
async def get_lyrics(id: int):
    url = f"https://api.tidal.com/v1/tracks/{id}/lyrics"
    data, _, _ = await authed_get_json(
        url,
        params={"countryCode": COUNTRY_CODE, "locale": "en_US", "deviceType": "BROWSER"},
    )

    if not data:
        raise HTTPException(status_code=404, detail="Lyrics not found")

    return {"version": API_VERSION, "lyrics": data}


# =============================================================================
# Bulk Download Endpoints
# =============================================================================

@app.post("/batch/tracks/", response_model=BatchTrackResponse)
async def batch_get_tracks(
    request: BatchTrackRequest,
    concurrency: int = Query(default=BULK_DEFAULT_CONCURRENCY, ge=1, le=BULK_MAX_CONCURRENCY),
):
    """
    Batch resolve streaming URLs for multiple tracks.

    - **track_ids**: List of Tidal track IDs (max 500 per request)
    - **quality**: Audio quality (HI_RES_LOSSLESS, LOSSLESS, HIGH, LOW)
    - **concurrency**: Number of parallel requests (1-30)

    Returns streaming URLs for all tracks with error details for any failures.
    """
    if len(request.track_ids) > BULK_MAX_TRACKS_PER_REQUEST:
        raise HTTPException(
            status_code=400,
            detail=f"Maximum {BULK_MAX_TRACKS_PER_REQUEST} tracks per request"
        )

    if not request.track_ids:
        raise HTTPException(status_code=400, detail="track_ids cannot be empty")

    results = await batch_fetch_tracks(request.track_ids, request.quality, concurrency)

    success_count = sum(1 for r in results if r.success)
    failed_count = len(results) - success_count

    return BatchTrackResponse(
        version=API_VERSION,
        results=results,
        stats=BatchStats(
            total=len(results),
            success=success_count,
            failed=failed_count
        )
    )


@app.get("/batch/tracks/stream")
async def batch_tracks_stream(
    track_ids: str = Query(..., description="Comma-separated track IDs"),
    quality: AudioQuality = Query(default=AudioQuality.HI_RES_LOSSLESS),
    concurrency: int = Query(default=BULK_DEFAULT_CONCURRENCY, ge=1, le=BULK_MAX_CONCURRENCY),
):
    """
    Stream track URL resolution progress via Server-Sent Events.

    Returns events in the format:
    - `{"type": "track", "track_id": int, "success": bool, "data": {...}, "progress": int, "total": int}`
    - `{"type": "complete", "stats": {"total": int, "success": int, "failed": int}}`
    """
    try:
        ids = [int(x.strip()) for x in track_ids.split(",") if x.strip()]
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid track_ids format")

    if not ids:
        raise HTTPException(status_code=400, detail="track_ids cannot be empty")

    if len(ids) > BULK_MAX_TRACKS_PER_REQUEST:
        raise HTTPException(
            status_code=400,
            detail=f"Maximum {BULK_MAX_TRACKS_PER_REQUEST} tracks per request"
        )

    async def event_generator():
        total = len(ids)
        sem = asyncio.Semaphore(concurrency)

        async def fetch_one(track_id: int, cred: dict):
            async with sem:
                try:
                    data = await fetch_track_with_retry(track_id, quality, cred)
                    return {"track_id": track_id, "success": True, "data": data}
                except Exception as e:
                    return {"track_id": track_id, "success": False, "error": str(e)[:200]}

        # Create tasks with round-robin credential assignment
        tasks = []
        for i, track_id in enumerate(ids):
            cred = _creds[i % len(_creds)] if _creds else _pick_credential()
            tasks.append(fetch_one(track_id, cred))

        # Process and yield results as they complete, counting in the single consumer
        completed = 0
        success_count = 0
        for coro in asyncio.as_completed(tasks):
            result = await coro
            completed += 1
            if result["success"]:
                success_count += 1
            yield f"data: {json.dumps({'type': 'track', **result, 'progress': completed, 'total': total})}\n\n"

        # Final completion event
        yield f"data: {json.dumps({'type': 'complete', 'stats': {'total': total, 'success': success_count, 'failed': total - success_count}})}\n\n"

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        }
    )


@app.get("/batch/album/{album_id}/tracks")
async def batch_album_tracks(
    album_id: int,
    quality: AudioQuality = Query(default=AudioQuality.HI_RES_LOSSLESS),
    concurrency: int = Query(default=BULK_DEFAULT_CONCURRENCY, ge=1, le=BULK_MAX_CONCURRENCY),
):
    """
    Get album metadata and resolve all track streaming URLs in one call.

    Returns album info plus streaming URLs for all tracks.
    """
    # Fetch album metadata and track list
    album_url = f"https://api.tidal.com/v1/albums/{album_id}"
    items_url = f"https://api.tidal.com/v1/albums/{album_id}/items"

    async def fetch(url: str, params: Optional[dict] = None):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    # Fetch album info and all items
    album_data, items_data = await asyncio.gather(
        fetch(album_url, {"countryCode": COUNTRY_CODE}),
        fetch(items_url, {"countryCode": COUNTRY_CODE, "limit": 500, "offset": 0}),
    )

    items = items_data.get("items", [])
    track_ids = [
        item.get("item", item).get("id")
        for item in items
        if item.get("item", item).get("type", "track") == "track"
    ]

    # Batch fetch all track URLs
    track_results = await batch_fetch_tracks(track_ids, quality, concurrency)

    # Create a map of track_id -> streaming data
    track_url_map = {r.track_id: r for r in track_results}

    # Enrich items with streaming URLs
    enriched_items = []
    for item in items:
        track = item.get("item", item)
        track_id = track.get("id")
        result = track_url_map.get(track_id)
        enriched_items.append({
            "track": track,
            "playback": result.data if result and result.success else None,
            "error": result.error if result and not result.success else None,
        })

    success_count = sum(1 for r in track_results if r.success)

    return {
        "version": API_VERSION,
        "album": album_data,
        "tracks": enriched_items,
        "stats": {
            "total": len(track_results),
            "success": success_count,
            "failed": len(track_results) - success_count
        }
    }


@app.get("/batch/playlist/{playlist_id}/tracks")
async def batch_playlist_tracks(
    playlist_id: str,
    quality: AudioQuality = Query(default=AudioQuality.HI_RES_LOSSLESS),
    concurrency: int = Query(default=BULK_DEFAULT_CONCURRENCY, ge=1, le=BULK_MAX_CONCURRENCY),
    limit: int = Query(default=500, ge=1, le=500),
    offset: int = Query(default=0, ge=0),
):
    """
    Get playlist metadata and resolve all track streaming URLs in one call.

    Returns playlist info plus streaming URLs for all tracks.
    """
    playlist_url = f"https://api.tidal.com/v1/playlists/{playlist_id}"
    items_url = f"https://api.tidal.com/v1/playlists/{playlist_id}/items"

    async def fetch(url: str, params: Optional[dict] = None):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    playlist_data, items_data = await asyncio.gather(
        fetch(playlist_url, {"countryCode": COUNTRY_CODE}),
        fetch(items_url, {"countryCode": COUNTRY_CODE, "limit": limit, "offset": offset}),
    )

    items = items_data.get("items", [])
    track_ids = [
        item.get("item", item).get("id")
        for item in items
        if item.get("item", item).get("type", "track") == "track"
    ]

    track_results = await batch_fetch_tracks(track_ids, quality, concurrency)
    track_url_map = {r.track_id: r for r in track_results}

    enriched_items = []
    for item in items:
        track = item.get("item", item)
        track_id = track.get("id")
        result = track_url_map.get(track_id)
        enriched_items.append({
            "track": track,
            "playback": result.data if result and result.success else None,
            "error": result.error if result and not result.success else None,
        })

    success_count = sum(1 for r in track_results if r.success)

    return {
        "version": API_VERSION,
        "playlist": playlist_data,
        "tracks": enriched_items,
        "stats": {
            "total": len(track_results),
            "success": success_count,
            "failed": len(track_results) - success_count
        }
    }


@app.post("/batch/artist/{artist_id}/discography")
async def batch_artist_discography(
    artist_id: int,
    request: BatchArtistRequest = None,
    quality: AudioQuality = Query(default=AudioQuality.HI_RES_LOSSLESS),
    concurrency: int = Query(default=BULK_DEFAULT_CONCURRENCY, ge=1, le=BULK_MAX_CONCURRENCY),
    include_albums: bool = Query(default=True),
    include_eps: bool = Query(default=True),
    include_singles: bool = Query(default=True),
):
    """
    Fetch all albums for an artist and resolve all track streaming URLs.

    This can return a large amount of data for prolific artists.
    Consider using the job queue for very large discographies.
    """
    # Use request body if provided, otherwise use query params
    if request:
        quality = request.quality
        include_albums = request.include_albums
        include_eps = request.include_eps
        include_singles = request.include_singles

    albums_url = f"https://api.tidal.com/v1/artists/{artist_id}/albums"
    common_params = {"countryCode": COUNTRY_CODE, "limit": 100}

    async def fetch(url: str, params: Optional[dict] = None):
        payload, _, _ = await authed_get_json(url, params=params)
        return payload

    # Fetch albums and EPs/Singles in parallel
    tasks = []
    if include_albums:
        tasks.append(fetch(albums_url, common_params))
    if include_eps or include_singles:
        tasks.append(fetch(albums_url, {**common_params, "filter": "EPSANDSINGLES"}))

    results = await asyncio.gather(*tasks, return_exceptions=True)

    # Collect unique albums
    unique_albums = []
    seen_ids = set()

    for res in results:
        if isinstance(res, Exception):
            logger.error("Error fetching artist albums: %s", res)
            continue
        for item in res.get("items", []):
            album_id = item.get("id")
            if album_id and album_id not in seen_ids:
                unique_albums.append(item)
                seen_ids.add(album_id)

    # Fetch tracks for each album
    sem = asyncio.Semaphore(6)  # Limit concurrent album fetches

    async def fetch_album_tracks(album_id: int):
        async with sem:
            album_data, _, _ = await authed_get_json(
                "https://api.tidal.com/v1/pages/album",
                params={
                    "albumId": album_id,
                    "countryCode": COUNTRY_CODE,
                    "deviceType": "BROWSER",
                },
            )

            rows = album_data.get("rows", [])
            if len(rows) < 2:
                return []
            modules = rows[1].get("modules", [])
            if not modules:
                return []
            paged_list = modules[0].get("pagedList", {})
            items = paged_list.get("items", [])
            return [track.get("item", track) for track in items]

    album_track_results = await asyncio.gather(
        *(fetch_album_tracks(a["id"]) for a in unique_albums),
        return_exceptions=True,
    )

    # Collect all track IDs
    all_tracks = []
    for res in album_track_results:
        if isinstance(res, Exception):
            continue
        all_tracks.extend(res)

    track_ids = [t.get("id") for t in all_tracks if t.get("id")]

    # Batch fetch all track URLs
    track_results = await batch_fetch_tracks(track_ids, quality, concurrency)
    track_url_map = {r.track_id: r for r in track_results}

    # Enrich tracks with streaming URLs
    enriched_tracks = []
    for track in all_tracks:
        track_id = track.get("id")
        result = track_url_map.get(track_id)
        enriched_tracks.append({
            "track": track,
            "playback": result.data if result and result.success else None,
            "error": result.error if result and not result.success else None,
        })

    success_count = sum(1 for r in track_results if r.success)

    return {
        "version": API_VERSION,
        "artist_id": artist_id,
        "albums": unique_albums,
        "tracks": enriched_tracks,
        "stats": {
            "total_albums": len(unique_albums),
            "total_tracks": len(track_results),
            "success": success_count,
            "failed": len(track_results) - success_count
        }
    }


# =============================================================================
# Job Queue Endpoints for Large Batches
# =============================================================================

@app.post("/jobs/create")
async def create_job(request: BatchTrackRequest):
    """
    Create an async job for processing large batches of tracks.

    Returns a job_id that can be used to poll for status and results.
    Use this for batches larger than 500 tracks or when you don't need immediate results.
    """
    job_id = str(uuid.uuid4())
    # Evict old completed jobs (keep max 100, remove completed jobs older than 1 hour)
    async with _jobs_lock:
        _evict_stale_jobs()

    job = DownloadJob(
        job_id=job_id,
        status="pending",
        track_ids=request.track_ids,
        quality=request.quality,
        created_at=datetime.now(timezone.utc),
        total=len(request.track_ids),
    )

    async with _jobs_lock:
        _jobs[job_id] = job

    # Start background processing with error handling
    task = asyncio.create_task(_process_job(job))
    task.add_done_callback(_job_task_done)

    return {
        "version": API_VERSION,
        "job_id": job_id,
        "status": "pending",
        "total": len(request.track_ids)
    }


_JOB_MAX_AGE_SECONDS = 3600  # 1 hour
_JOB_MAX_COUNT = 100


def _evict_stale_jobs():
    """Remove completed jobs older than max age. Must be called under _jobs_lock."""
    now = datetime.now(timezone.utc)
    to_delete = []
    for jid, job in _jobs.items():
        if job.status in ("completed", "failed") and job.completed_at:
            age = (now - job.completed_at).total_seconds()
            if age > _JOB_MAX_AGE_SECONDS:
                to_delete.append(jid)
    for jid in to_delete:
        del _jobs[jid]
    # If still over limit, remove oldest completed jobs
    if len(_jobs) > _JOB_MAX_COUNT:
        completed = sorted(
            [(jid, j) for jid, j in _jobs.items() if j.status in ("completed", "failed")],
            key=lambda x: x[1].created_at,
        )
        while len(_jobs) > _JOB_MAX_COUNT and completed:
            jid, _ = completed.pop(0)
            del _jobs[jid]


def _job_task_done(task: asyncio.Task):
    """Callback for background job tasks to log unhandled exceptions."""
    if task.cancelled():
        return
    exc = task.exception()
    if exc:
        logger.error("Background job failed: %s", exc, exc_info=exc)


async def _process_job(job: DownloadJob):
    """Background task to process a download job."""
    job.status = "processing"

    sem = asyncio.Semaphore(BULK_DEFAULT_CONCURRENCY)

    async def process_track(track_id: int, cred: dict):
        async with sem:
            try:
                result = await fetch_track_with_retry(track_id, job.quality, cred)
                job.results[track_id] = result
            except Exception as e:
                job.errors[track_id] = str(e)[:200]
            finally:
                job.progress += 1

    # Round-robin credential assignment
    tasks = []
    for i, track_id in enumerate(job.track_ids):
        cred = _creds[i % len(_creds)] if _creds else _pick_credential()
        tasks.append(process_track(track_id, cred))

    await asyncio.gather(*tasks, return_exceptions=True)

    job.status = "completed"
    job.completed_at = datetime.now(timezone.utc)


# IMPORTANT: /jobs/ list must be defined before /jobs/{job_id} to avoid route conflicts
@app.get("/jobs/")
async def list_jobs():
    """List all active jobs."""
    return {
        "version": API_VERSION,
        "jobs": [
            {
                "job_id": job.job_id,
                "status": job.status,
                "progress": job.progress,
                "total": job.total,
                "created_at": job.created_at.isoformat(),
            }
            for job in _jobs.values()
        ]
    }


@app.get("/jobs/{job_id}")
async def get_job_status(job_id: str):
    """Get the status and progress of a job."""
    job = _jobs.get(job_id)
    if not job:
        raise HTTPException(status_code=404, detail="Job not found")

    return {
        "version": API_VERSION,
        "job_id": job.job_id,
        "status": job.status,
        "progress": job.progress,
        "total": job.total,
        "success": len(job.results),
        "failed": len(job.errors),
        "created_at": job.created_at.isoformat(),
        "completed_at": job.completed_at.isoformat() if job.completed_at else None,
    }


@app.get("/jobs/{job_id}/results")
async def get_job_results(job_id: str):
    """Get the results of a completed job."""
    job = _jobs.get(job_id)
    if not job:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.status != "completed":
        raise HTTPException(
            status_code=400,
            detail=f"Job is not completed (status: {job.status})"
        )

    return {
        "version": API_VERSION,
        "job_id": job.job_id,
        "results": job.results,
        "errors": job.errors,
        "stats": {
            "total": job.total,
            "success": len(job.results),
            "failed": len(job.errors)
        }
    }


@app.delete("/jobs/{job_id}")
async def delete_job(job_id: str):
    """Delete/cleanup a job."""
    async with _jobs_lock:
        if job_id not in _jobs:
            raise HTTPException(status_code=404, detail="Job not found")
        del _jobs[job_id]

    return {"version": API_VERSION, "message": "Job deleted", "job_id": job_id}


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
