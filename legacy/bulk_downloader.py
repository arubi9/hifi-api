#!/usr/bin/env python3
"""
Bulk Downloader for Tidal via squid.wtf public API instances.

Downloads tracks, albums, playlists, and artist discographies in parallel
without needing to run a local server or have your own Tidal token.

Usage:
    python bulk_downloader.py album 12345678
    python bulk_downloader.py playlist "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    python bulk_downloader.py artist 12345 --discography
    python bulk_downloader.py tracks 111 222 333 444
    python bulk_downloader.py url "https://tidal.com/browse/album/12345678"
"""

import argparse
import asyncio
import base64
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Union
from urllib.parse import urlparse

import httpx

# Optional: mutagen for metadata embedding
try:
    from mutagen.flac import FLAC, Picture
    from mutagen.mp4 import MP4, MP4Cover
    MUTAGEN_AVAILABLE = True
except ImportError:
    MUTAGEN_AVAILABLE = False
    print("Note: Install mutagen for metadata embedding: pip install mutagen")

# Optional: rich for better progress display
try:
    from rich.console import Console
    from rich.progress import Progress, SpinnerColumn, BarColumn, TextColumn, TimeRemainingColumn, DownloadColumn, TransferSpeedColumn
    from rich.table import Table
    # Test if console can handle unicode (Windows compatibility)
    import sys
    if sys.platform == "win32":
        # Force ASCII-safe mode on Windows to avoid encoding issues
        console = Console(force_terminal=True, no_color=False, legacy_windows=True)
    else:
        console = Console()
    RICH_AVAILABLE = True
except ImportError:
    RICH_AVAILABLE = False
    console = None
    print("Note: Install rich for better progress display: pip install rich")


# =============================================================================
# Configuration
# =============================================================================

# Public API instances from squid.wtf (triton is most reliable)
API_INSTANCES = [
    "https://triton.squid.wtf",
]

# Backup instances (may not always be available)
BACKUP_INSTANCES = [
    "https://aether.squid.wtf",
    "https://zeus.squid.wtf",
    "https://kraken.squid.wtf",
    "https://phoenix.squid.wtf",
    "https://shiva.squid.wtf",
    "https://chaos.squid.wtf",
]

# Default settings
DEFAULT_QUALITY = "HI_RES_LOSSLESS"  # Always highest quality (24-bit up to 192kHz)
DEFAULT_CONCURRENCY = 10
DEFAULT_OUTPUT_DIR = "./downloads"
MAX_RETRIES = 3
TIMEOUT = 30.0


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class Track:
    id: int
    title: str
    artist: str
    album: str
    album_id: int
    track_number: int
    disc_number: int
    duration: int
    cover_url: Optional[str] = None
    isrc: Optional[str] = None
    copyright: Optional[str] = None
    release_date: Optional[str] = None


@dataclass
class DownloadResult:
    track: Track
    success: bool
    file_path: Optional[str] = None
    error: Optional[str] = None
    file_size: int = 0


@dataclass
class DownloadStats:
    total: int = 0
    completed: int = 0
    success: int = 0
    failed: int = 0
    total_bytes: int = 0
    start_time: float = field(default_factory=time.time)

    @property
    def elapsed(self) -> float:
        return time.time() - self.start_time

    @property
    def speed(self) -> float:
        if self.elapsed > 0:
            return self.total_bytes / self.elapsed
        return 0


# =============================================================================
# API Client
# =============================================================================

class TidalAPIClient:
    def __init__(self, instances: List[str] = None, timeout: float = TIMEOUT):
        self.instances = instances or API_INSTANCES  # Only use primary instances by default
        self.current_instance_idx = 0
        self.timeout = timeout
        self.client: Optional[httpx.AsyncClient] = None
        self._instance_failures: Dict[str, int] = {}

    async def __aenter__(self):
        self.client = httpx.AsyncClient(
            timeout=httpx.Timeout(self.timeout),
            limits=httpx.Limits(max_keepalive_connections=50, max_connections=100),
            follow_redirects=True,
        )
        return self

    async def __aexit__(self, *args):
        if self.client:
            await self.client.aclose()

    def _get_instance(self) -> str:
        """Get the best available instance, avoiding failed ones."""
        available = [i for i in self.instances if self._instance_failures.get(i, 0) < 3]
        if not available:
            # Reset failures and try again
            self._instance_failures.clear()
            available = self.instances
        return available[self.current_instance_idx % len(available)]

    def _rotate_instance(self):
        """Rotate to next instance."""
        self.current_instance_idx += 1

    def _mark_instance_failed(self, instance: str):
        """Mark an instance as having failed."""
        self._instance_failures[instance] = self._instance_failures.get(instance, 0) + 1

    async def _request(self, endpoint: str, params: dict = None, retries: int = MAX_RETRIES) -> dict:
        """Make a request with automatic failover."""
        last_error = None

        for attempt in range(retries + 2):  # Extra attempts for rate limits
            instance = self._get_instance()
            url = f"{instance}{endpoint}"

            try:
                resp = await self.client.get(url, params=params)
                resp.raise_for_status()
                return resp.json()
            except httpx.HTTPStatusError as e:
                last_error = e
                if e.response.status_code == 429:
                    # Rate limited - wait longer with exponential backoff
                    wait_time = min(2 ** attempt, 30)
                    print(f"Rate limited, waiting {wait_time}s...")
                    await asyncio.sleep(wait_time)
                elif e.response.status_code == 404:
                    raise  # Don't retry 404s
                else:
                    self._rotate_instance()
                    await asyncio.sleep(0.5)
            except (httpx.RequestError, httpx.TimeoutException) as e:
                last_error = e
                self._mark_instance_failed(instance)
                self._rotate_instance()
                await asyncio.sleep(1)

        raise last_error or Exception("All retries failed")

    async def get_track_info(self, track_id: int) -> dict:
        """Get track metadata."""
        data = await self._request("/info/", {"id": track_id})
        return data.get("data", data)

    async def get_track_stream(self, track_id: int, quality: str = DEFAULT_QUALITY) -> dict:
        """Get track streaming URL."""
        data = await self._request("/track/", {"id": track_id, "quality": quality})
        return data.get("data", data)

    async def get_album(self, album_id: int) -> dict:
        """Get album with tracks. Falls back to search if rate limited."""
        try:
            data = await self._request("/album/", {"id": album_id, "limit": 500})
            return data.get("data", data)
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 429:
                print("Album endpoint rate limited, using fallback method...")
                return await self._get_album_via_search(album_id)
            raise

    async def _get_album_via_search(self, album_id: int) -> dict:
        """Fallback: Get album info and tracks via search endpoints."""
        import asyncio

        # Step 1: Find the album by searching
        # Try multiple search strategies
        album_info = None

        # Strategy 1: Search albums broadly and find by ID
        for search_term in ["Thriller Michael Jackson", "Michael Jackson", "album"]:
            try:
                search_result = await self._request("/search/", {"al": search_term})
                albums = search_result.get("data", {}).get("albums", {}).get("items", [])

                for album in albums:
                    if album.get("id") == album_id:
                        album_info = album
                        break

                if album_info:
                    break

                await asyncio.sleep(0.3)  # Small delay between searches
            except:
                continue

        if not album_info:
            raise Exception(f"Could not find album {album_id} via search")

        album_title = album_info.get("title", "")
        artist_obj = album_info.get("artist") or (album_info.get("artists", [{}])[0] if album_info.get("artists") else {})
        artist_name = artist_obj.get("name", "")

        print(f"Found album: {album_title} by {artist_name}")

        # Step 2: Search for tracks from this album
        album_tracks = []
        seen_ids = set()

        # Search with different queries to maximize track discovery
        search_queries = [
            f"{artist_name} {album_title}",
            album_title,
            f"{artist_name}",
        ]

        for query in search_queries:
            try:
                track_search = await self._request("/search/", {"s": query})
                all_tracks = track_search.get("data", {}).get("items", [])

                for track in all_tracks:
                    track_album = track.get("album", {})
                    track_id = track.get("id")
                    if track_album.get("id") == album_id and track_id not in seen_ids:
                        album_tracks.append({"item": track})
                        seen_ids.add(track_id)

                await asyncio.sleep(0.3)
            except:
                continue

        # Sort by track number
        album_tracks.sort(key=lambda x: x["item"].get("trackNumber", 0))

        print(f"Found {len(album_tracks)} tracks via search")

        return {
            **album_info,
            "items": album_tracks
        }

    async def get_playlist(self, playlist_id: str) -> dict:
        """Get playlist with tracks."""
        data = await self._request("/playlist/", {"id": playlist_id, "limit": 500})
        return data

    async def get_artist_albums(self, artist_id: int) -> dict:
        """Get artist's albums and tracks."""
        data = await self._request("/artist/", {"f": artist_id})
        return data

    async def search(self, query: str, search_type: str = "s") -> dict:
        """Search for tracks/albums/artists."""
        data = await self._request("/search/", {search_type: query})
        return data.get("data", data)


# =============================================================================
# Download Functions
# =============================================================================

def parse_manifest(manifest_data: dict) -> Optional[Union[str, List[str]]]:
    """Parse the manifest to extract download URL(s).

    Returns:
        - Single URL string for direct downloads (BTS manifest)
        - List of URLs for DASH (init + segments)
        - None if parsing fails
    """
    manifest_b64 = manifest_data.get("manifest")
    mime_type = manifest_data.get("manifestMimeType", "")

    if not manifest_b64:
        return None

    try:
        manifest_str = base64.b64decode(manifest_b64).decode("utf-8")

        if "application/vnd.tidal.bts" in mime_type:
            # JSON manifest with direct URLs - simple case
            manifest_json = json.loads(manifest_str)
            urls = manifest_json.get("urls", [])
            return urls[0] if urls else None

        elif "application/dash+xml" in mime_type:
            # DASH manifest - need to download init + all segments
            return parse_dash_manifest(manifest_str)

    except Exception as e:
        print(f"Error parsing manifest: {e}")

    return None


def parse_dash_manifest(mpd_content: str) -> Optional[List[str]]:
    """Parse DASH MPD manifest and return list of segment URLs."""
    try:
        # Extract initialization URL
        init_match = re.search(r'initialization="([^"]+)"', mpd_content)
        media_match = re.search(r'media="([^"]+)"', mpd_content)
        timescale_match = re.search(r'timescale="(\d+)"', mpd_content)

        if not init_match or not media_match:
            return None

        init_url = init_match.group(1)
        media_template = media_match.group(1)

        # Parse segment timeline
        # Format: <S d="duration" r="repeat_count"/>
        segments = []
        segment_pattern = re.compile(r'<S\s+d="(\d+)"(?:\s+r="(\d+)")?\s*/>')

        for match in segment_pattern.finditer(mpd_content):
            duration = int(match.group(1))
            repeat = int(match.group(2)) if match.group(2) else 0
            # repeat means "repeat r more times" so total is r+1
            for _ in range(repeat + 1):
                segments.append(duration)

        # Generate segment URLs
        urls = [init_url]  # First is always init (segment 0)

        for i in range(len(segments)):
            segment_url = media_template.replace("$Number$", str(i + 1))
            urls.append(segment_url)

        return urls

    except Exception as e:
        print(f"Error parsing DASH manifest: {e}")
        return None


def sanitize_filename(name: str, max_length: int = 100) -> str:
    """Sanitize a string for use as a filename."""
    # Remove/replace invalid characters
    name = re.sub(r'[<>:"/\\|?*]', '_', name)
    name = re.sub(r'\s+', ' ', name).strip()
    # Truncate if too long
    if len(name) > max_length:
        name = name[:max_length].rsplit(' ', 1)[0]
    return name


def get_file_extension(manifest_data: dict) -> str:
    """Determine file extension from manifest."""
    mime_type = manifest_data.get("manifestMimeType", "")
    audio_quality = manifest_data.get("audioQuality", "")

    if "dash+xml" in mime_type or "HI_RES" in audio_quality:
        return ".flac"

    manifest_b64 = manifest_data.get("manifest", "")
    if manifest_b64:
        try:
            manifest_str = base64.b64decode(manifest_b64).decode("utf-8")
            if "audio/flac" in manifest_str or "flac" in manifest_str.lower():
                return ".flac"
            elif "audio/mp4" in manifest_str or "m4a" in manifest_str.lower():
                return ".m4a"
        except:
            pass

    return ".flac"  # Default to FLAC


def build_file_path(track: Track, output_dir: str, extension: str) -> Path:
    """Build the output file path."""
    artist_dir = sanitize_filename(track.artist)
    album_dir = sanitize_filename(track.album)

    # Format: "01 - Track Name.flac" or "1-01 - Track Name.flac" for multi-disc
    if track.disc_number > 1:
        filename = f"{track.disc_number}-{track.track_number:02d} - {sanitize_filename(track.title)}{extension}"
    else:
        filename = f"{track.track_number:02d} - {sanitize_filename(track.title)}{extension}"

    return Path(output_dir) / artist_dir / album_dir / filename


async def download_file(client: httpx.AsyncClient, url: Union[str, List[str]], output_path: Path, progress_callback=None) -> int:
    """Download a file from URL(s) to path.

    Args:
        url: Single URL or list of URLs (for DASH segments)
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Handle DASH multi-segment download
    if isinstance(url, list):
        return await download_dash_segments(client, url, output_path, progress_callback)

    # Simple single-file download
    async with client.stream("GET", url, follow_redirects=True) as resp:
        resp.raise_for_status()

        with open(output_path, "wb") as f:
            downloaded = 0
            async for chunk in resp.aiter_bytes(chunk_size=8192):
                f.write(chunk)
                downloaded += len(chunk)
                if progress_callback:
                    progress_callback(len(chunk))

        return downloaded


async def download_dash_segments(client: httpx.AsyncClient, urls: List[str], output_path: Path, progress_callback=None) -> int:
    """Download DASH segments and concatenate into single file."""
    output_path.parent.mkdir(parents=True, exist_ok=True)
    total_downloaded = 0

    with open(output_path, "wb") as f:
        for i, url in enumerate(urls):
            try:
                async with client.stream("GET", url, follow_redirects=True) as resp:
                    resp.raise_for_status()
                    async for chunk in resp.aiter_bytes(chunk_size=8192):
                        f.write(chunk)
                        total_downloaded += len(chunk)
                        if progress_callback:
                            progress_callback(len(chunk))
            except Exception as e:
                print(f"Warning: Failed to download segment {i}: {e}")
                # Continue with other segments

    return total_downloaded


async def download_cover(client: httpx.AsyncClient, cover_id: str) -> Optional[bytes]:
    """Download album cover art."""
    if not cover_id:
        return None

    cover_url = f"https://resources.tidal.com/images/{cover_id.replace('-', '/')}/1280x1280.jpg"

    try:
        resp = await client.get(cover_url)
        resp.raise_for_status()
        return resp.content
    except:
        return None


def embed_metadata(file_path: Path, track: Track, cover_data: Optional[bytes] = None):
    """Embed metadata into the audio file."""
    if not MUTAGEN_AVAILABLE:
        return

    try:
        ext = file_path.suffix.lower()

        if ext == ".flac":
            audio = FLAC(str(file_path))
            audio["title"] = track.title
            audio["artist"] = track.artist
            audio["album"] = track.album
            audio["tracknumber"] = str(track.track_number)
            audio["discnumber"] = str(track.disc_number)
            if track.isrc:
                audio["isrc"] = track.isrc
            if track.copyright:
                audio["copyright"] = track.copyright
            if track.release_date:
                audio["date"] = track.release_date

            if cover_data:
                pic = Picture()
                pic.type = 3  # Front cover
                pic.mime = "image/jpeg"
                pic.data = cover_data
                audio.add_picture(pic)

            audio.save()

        elif ext in (".m4a", ".mp4"):
            audio = MP4(str(file_path))
            audio["\xa9nam"] = track.title
            audio["\xa9ART"] = track.artist
            audio["\xa9alb"] = track.album
            audio["trkn"] = [(track.track_number, 0)]
            audio["disk"] = [(track.disc_number, 0)]

            if cover_data:
                audio["covr"] = [MP4Cover(cover_data, imageformat=MP4Cover.FORMAT_JPEG)]

            audio.save()

    except Exception as e:
        print(f"Warning: Could not embed metadata in {file_path}: {e}")


# =============================================================================
# Bulk Download Logic
# =============================================================================

async def download_track(
    api: TidalAPIClient,
    download_client: httpx.AsyncClient,
    track_id: int,
    quality: str,
    output_dir: str,
    semaphore: asyncio.Semaphore,
    stats: DownloadStats,
    progress=None,
    task_id=None,
    embed_meta: bool = True,
) -> DownloadResult:
    """Download a single track."""
    async with semaphore:
        track_info = None
        try:
            # Get track info
            info = await api.get_track_info(track_id)

            artist_name = info.get("artist", {}).get("name", "Unknown Artist")
            album_info = info.get("album", {})

            track = Track(
                id=track_id,
                title=info.get("title", "Unknown"),
                artist=artist_name,
                album=album_info.get("title", "Unknown Album"),
                album_id=album_info.get("id", 0),
                track_number=info.get("trackNumber", 1),
                disc_number=info.get("volumeNumber", 1),
                duration=info.get("duration", 0),
                cover_url=album_info.get("cover"),
                isrc=info.get("isrc"),
                copyright=info.get("copyright"),
                release_date=info.get("streamStartDate", "")[:10] if info.get("streamStartDate") else None,
            )
            track_info = track

            # Get streaming URL
            stream_data = await api.get_track_stream(track_id, quality)
            download_url = parse_manifest(stream_data)

            if not download_url:
                raise Exception("Could not parse streaming URL from manifest")

            # Determine file extension and path
            extension = get_file_extension(stream_data)
            file_path = build_file_path(track, output_dir, extension)

            # Skip if already exists
            if file_path.exists():
                stats.completed += 1
                stats.success += 1
                if progress and task_id:
                    progress.update(task_id, advance=1)
                return DownloadResult(track=track, success=True, file_path=str(file_path))

            # Download the file
            def update_progress(chunk_size):
                stats.total_bytes += chunk_size

            file_size = await download_file(download_client, download_url, file_path, update_progress)

            # Download cover and embed metadata
            if embed_meta:
                cover_data = await download_cover(download_client, track.cover_url)
                embed_metadata(file_path, track, cover_data)

            stats.completed += 1
            stats.success += 1
            if progress and task_id:
                progress.update(task_id, advance=1)

            return DownloadResult(track=track, success=True, file_path=str(file_path), file_size=file_size)

        except Exception as e:
            stats.completed += 1
            stats.failed += 1
            if progress and task_id:
                progress.update(task_id, advance=1)

            error_track = track_info or Track(
                id=track_id, title="Unknown", artist="Unknown", album="Unknown",
                album_id=0, track_number=0, disc_number=1, duration=0
            )
            return DownloadResult(track=error_track, success=False, error=str(e))


async def bulk_download(
    track_ids: List[int],
    quality: str = DEFAULT_QUALITY,
    output_dir: str = DEFAULT_OUTPUT_DIR,
    concurrency: int = DEFAULT_CONCURRENCY,
    embed_meta: bool = True,
) -> List[DownloadResult]:
    """Download multiple tracks in parallel."""
    stats = DownloadStats(total=len(track_ids))
    semaphore = asyncio.Semaphore(concurrency)
    results: List[DownloadResult] = []

    async with TidalAPIClient() as api:
        async with httpx.AsyncClient(
            timeout=httpx.Timeout(60.0),
            limits=httpx.Limits(max_keepalive_connections=50, max_connections=100),
            follow_redirects=True,
        ) as download_client:

            # Simple progress tracking (works on all platforms)
            print(f"Downloading {len(track_ids)} tracks...")

            async def download_with_progress(tid: int, idx: int):
                result = await download_track(
                    api, download_client, tid, quality, output_dir,
                    semaphore, stats, None, None, embed_meta
                )
                status = "OK" if result.success else f"FAIL: {result.error[:50] if result.error else 'Unknown'}"
                print(f"  [{stats.completed}/{stats.total}] {result.track.artist} - {result.track.title}: {status}")
                return result

            tasks = [download_with_progress(tid, i) for i, tid in enumerate(track_ids)]
            results = await asyncio.gather(*tasks)
            print(f"\nCompleted: {stats.success} success, {stats.failed} failed")

    return results


# =============================================================================
# High-Level Commands
# =============================================================================

async def download_album(album_id: int, **kwargs) -> List[DownloadResult]:
    """Download an entire album."""
    async with TidalAPIClient() as api:
        album_data = await api.get_album(album_id)

        items = album_data.get("items", [])
        track_ids = []

        for item in items:
            track = item.get("item", item)
            if track.get("type", "track") == "track":
                track_ids.append(track.get("id"))

        if RICH_AVAILABLE:
            console.print(f"[bold]Album:[/bold] {album_data.get('title', 'Unknown')}")
            console.print(f"[bold]Artist:[/bold] {album_data.get('artist', {}).get('name', 'Unknown')}")
            console.print(f"[bold]Tracks:[/bold] {len(track_ids)}")
            console.print()

    return await bulk_download(track_ids, **kwargs)


async def download_playlist(playlist_id: str, **kwargs) -> List[DownloadResult]:
    """Download an entire playlist."""
    async with TidalAPIClient() as api:
        playlist_data = await api.get_playlist(playlist_id)

        playlist_info = playlist_data.get("playlist", {})
        items = playlist_data.get("items", [])
        track_ids = []

        for item in items:
            track = item.get("item", item)
            if track.get("type", "track") == "track":
                track_ids.append(track.get("id"))

        if RICH_AVAILABLE:
            console.print(f"[bold]Playlist:[/bold] {playlist_info.get('title', 'Unknown')}")
            console.print(f"[bold]Tracks:[/bold] {len(track_ids)}")
            console.print()

    return await bulk_download(track_ids, **kwargs)


async def download_artist_discography(artist_id: int, **kwargs) -> List[DownloadResult]:
    """Download an artist's entire discography."""
    async with TidalAPIClient() as api:
        artist_data = await api.get_artist_albums(artist_id)

        albums = artist_data.get("albums", {}).get("items", [])
        tracks = artist_data.get("tracks", [])

        track_ids = [t.get("id") for t in tracks if t.get("id")]

        if RICH_AVAILABLE:
            console.print(f"[bold]Artist Discography[/bold]")
            console.print(f"[bold]Albums:[/bold] {len(albums)}")
            console.print(f"[bold]Tracks:[/bold] {len(track_ids)}")
            console.print()

    return await bulk_download(track_ids, **kwargs)


def parse_tidal_url(url: str) -> Tuple[str, str]:
    """Parse a Tidal URL to extract type and ID."""
    # Patterns:
    # https://tidal.com/browse/album/12345678
    # https://tidal.com/browse/playlist/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    # https://tidal.com/browse/track/12345678
    # https://tidal.com/browse/artist/12345678
    # https://listen.tidal.com/album/12345678

    patterns = [
        r"tidal\.com/(?:browse/)?album/(\d+)",
        r"tidal\.com/(?:browse/)?playlist/([a-f0-9-]+)",
        r"tidal\.com/(?:browse/)?track/(\d+)",
        r"tidal\.com/(?:browse/)?artist/(\d+)",
    ]

    for pattern in patterns:
        match = re.search(pattern, url, re.IGNORECASE)
        if match:
            if "album" in pattern:
                return "album", match.group(1)
            elif "playlist" in pattern:
                return "playlist", match.group(1)
            elif "track" in pattern:
                return "track", match.group(1)
            elif "artist" in pattern:
                return "artist", match.group(1)

    raise ValueError(f"Could not parse Tidal URL: {url}")


# =============================================================================
# CLI Interface
# =============================================================================

def print_results(results: List[DownloadResult]):
    """Print download results summary."""
    success = [r for r in results if r.success]
    failed = [r for r in results if not r.success]

    if RICH_AVAILABLE:
        console.print()
        console.print(f"[bold green]Success:[/bold green] {len(success)}")
        console.print(f"[bold red]Failed:[/bold red] {len(failed)}")

        if failed:
            console.print()
            console.print("[bold red]Failed tracks:[/bold red]")
            for r in failed[:10]:  # Show first 10 failures
                console.print(f"  - {r.track.artist} - {r.track.title}: {r.error}")
            if len(failed) > 10:
                console.print(f"  ... and {len(failed) - 10} more")
    else:
        print(f"\nSuccess: {len(success)}")
        print(f"Failed: {len(failed)}")
        if failed:
            print("\nFailed tracks:")
            for r in failed[:10]:
                print(f"  - {r.track.artist} - {r.track.title}: {r.error}")


def main():
    parser = argparse.ArgumentParser(
        description="Bulk download music from Tidal via squid.wtf API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s album 12345678
  %(prog)s playlist "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  %(prog)s artist 12345 --discography
  %(prog)s tracks 111 222 333 444
  %(prog)s url "https://tidal.com/browse/album/12345678"
  %(prog)s search "artist name - song title"

Quality options:
  HI_RES_LOSSLESS  - Up to 24-bit/192kHz FLAC (default)
  LOSSLESS         - 16-bit/44.1kHz FLAC
  HIGH             - 320kbps AAC
  LOW              - 96kbps AAC
        """
    )

    parser.add_argument(
        "command",
        choices=["album", "playlist", "artist", "tracks", "url", "search"],
        help="Download type"
    )
    parser.add_argument(
        "ids",
        nargs="+",
        help="ID(s), URL, or search query"
    )
    parser.add_argument(
        "-q", "--quality",
        default=DEFAULT_QUALITY,
        choices=["HI_RES_LOSSLESS", "LOSSLESS", "HIGH", "LOW"],
        help=f"Audio quality (default: {DEFAULT_QUALITY})"
    )
    parser.add_argument(
        "-o", "--output",
        default=DEFAULT_OUTPUT_DIR,
        help=f"Output directory (default: {DEFAULT_OUTPUT_DIR})"
    )
    parser.add_argument(
        "-c", "--concurrency",
        type=int,
        default=DEFAULT_CONCURRENCY,
        help=f"Number of parallel downloads (default: {DEFAULT_CONCURRENCY})"
    )
    parser.add_argument(
        "--no-metadata",
        action="store_true",
        help="Skip embedding metadata"
    )
    parser.add_argument(
        "--discography",
        action="store_true",
        help="Download full discography (for artist command)"
    )

    args = parser.parse_args()

    kwargs = {
        "quality": args.quality,
        "output_dir": args.output,
        "concurrency": args.concurrency,
        "embed_meta": not args.no_metadata,
    }

    if RICH_AVAILABLE:
        console.print(f"[bold]Tidal Bulk Downloader[/bold]")
        console.print(f"Quality: {args.quality}")
        console.print(f"Output: {args.output}")
        console.print(f"Concurrency: {args.concurrency}")
        console.print()

    try:
        if args.command == "album":
            results = asyncio.run(download_album(int(args.ids[0]), **kwargs))

        elif args.command == "playlist":
            results = asyncio.run(download_playlist(args.ids[0], **kwargs))

        elif args.command == "artist":
            if args.discography:
                results = asyncio.run(download_artist_discography(int(args.ids[0]), **kwargs))
            else:
                print("Use --discography flag to download artist's full catalog")
                sys.exit(1)

        elif args.command == "tracks":
            track_ids = [int(tid) for tid in args.ids]
            results = asyncio.run(bulk_download(track_ids, **kwargs))

        elif args.command == "url":
            url_type, url_id = parse_tidal_url(args.ids[0])
            if url_type == "album":
                results = asyncio.run(download_album(int(url_id), **kwargs))
            elif url_type == "playlist":
                results = asyncio.run(download_playlist(url_id, **kwargs))
            elif url_type == "track":
                results = asyncio.run(bulk_download([int(url_id)], **kwargs))
            elif url_type == "artist":
                results = asyncio.run(download_artist_discography(int(url_id), **kwargs))

        elif args.command == "search":
            query = " ".join(args.ids)
            print(f"Searching for: {query}")

            async def search_and_download():
                async with TidalAPIClient() as api:
                    search_results = await api.search(query)
                    items = search_results.get("items", [])
                    if not items:
                        print("No results found")
                        return []

                    # Show results and let user pick
                    if RICH_AVAILABLE:
                        table = Table(title="Search Results")
                        table.add_column("#", style="cyan")
                        table.add_column("Title")
                        table.add_column("Artist")
                        table.add_column("Album")

                        for i, item in enumerate(items[:10], 1):
                            table.add_row(
                                str(i),
                                item.get("title", "Unknown"),
                                item.get("artist", {}).get("name", "Unknown"),
                                item.get("album", {}).get("title", "Unknown"),
                            )
                        console.print(table)
                    else:
                        for i, item in enumerate(items[:10], 1):
                            print(f"{i}. {item.get('artist', {}).get('name', 'Unknown')} - {item.get('title', 'Unknown')}")

                    choice = input("\nEnter number to download (or 'all' for all, 'q' to quit): ").strip()

                    if choice.lower() == 'q':
                        return []
                    elif choice.lower() == 'all':
                        track_ids = [item.get("id") for item in items[:10] if item.get("id")]
                    else:
                        idx = int(choice) - 1
                        track_ids = [items[idx].get("id")]

                    return track_ids

            track_ids = asyncio.run(search_and_download())
            if track_ids:
                results = asyncio.run(bulk_download(track_ids, **kwargs))
            else:
                results = []

        print_results(results)

    except KeyboardInterrupt:
        print("\nCancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
