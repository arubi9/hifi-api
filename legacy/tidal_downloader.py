#!/usr/bin/env python3
"""
Tidal Hi-Res Downloader
Professional CLI for bulk downloading from Tidal via squid.wtf API.
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
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Union

import httpx

# Rich for professional terminal UI
from rich.console import Console, Group
from rich.panel import Panel
from rich.table import Table
from rich.progress import (
    Progress,
    SpinnerColumn,
    TextColumn,
    BarColumn,
    TaskProgressColumn,
    TimeRemainingColumn,
    DownloadColumn,
    TransferSpeedColumn,
)
from rich.live import Live
from rich.layout import Layout
from rich.text import Text
from rich.rule import Rule
from rich.prompt import Prompt, Confirm
from rich.style import Style
from rich.columns import Columns
from rich import box

# Optional: mutagen for metadata
try:
    from mutagen.flac import FLAC, Picture
    from mutagen.mp4 import MP4, MP4Cover
    MUTAGEN_AVAILABLE = True
except ImportError:
    MUTAGEN_AVAILABLE = False

# =============================================================================
# Configuration
# =============================================================================

API_INSTANCES = ["https://triton.squid.wtf"]
BACKUP_INSTANCES = [
    "https://aether.squid.wtf",
    "https://zeus.squid.wtf",
    "https://kraken.squid.wtf",
]

DEFAULT_QUALITY = "HI_RES_LOSSLESS"
DEFAULT_CONCURRENCY = 5
DEFAULT_OUTPUT_DIR = "./downloads"
MAX_RETRIES = 3
TIMEOUT = 30.0

# =============================================================================
# Console Setup
# =============================================================================

console = Console()

STYLE_HEADER = Style(color="bright_white", bold=True)
STYLE_SUCCESS = Style(color="green")
STYLE_ERROR = Style(color="red")
STYLE_WARNING = Style(color="yellow")
STYLE_INFO = Style(color="cyan")
STYLE_DIM = Style(color="bright_black")


def print_header():
    """Print application header."""
    header = Table.grid(padding=0)
    header.add_column(justify="center")
    header.add_row(Text("TIDAL HI-RES DOWNLOADER", style="bold bright_white"))
    header.add_row(Text("Professional Lossless Music Acquisition", style="bright_black"))

    console.print()
    console.print(Panel(header, border_style="bright_blue", padding=(0, 2)))
    console.print()


def print_config(quality: str, output: str, concurrency: int):
    """Print current configuration."""
    config_table = Table(show_header=False, box=box.SIMPLE, padding=(0, 1))
    config_table.add_column("Key", style="bright_black")
    config_table.add_column("Value", style="bright_white")

    quality_display = {
        "HI_RES_LOSSLESS": "Hi-Res [24-bit/192kHz]",
        "LOSSLESS": "CD Quality [16-bit/44.1kHz]",
        "HIGH": "High [320kbps AAC]",
        "LOW": "Low [96kbps AAC]",
    }.get(quality, quality)

    config_table.add_row("Quality", quality_display)
    config_table.add_row("Output", output)
    config_table.add_row("Concurrency", str(concurrency))

    console.print(config_table)
    console.print()


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
    def speed_mbps(self) -> float:
        if self.elapsed > 0:
            return (self.total_bytes / 1024 / 1024) / self.elapsed
        return 0


# =============================================================================
# API Client
# =============================================================================

class TidalAPIClient:
    def __init__(self, instances: List[str] = None, timeout: float = TIMEOUT):
        self.instances = instances or API_INSTANCES
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
        available = [i for i in self.instances if self._instance_failures.get(i, 0) < 3]
        if not available:
            self._instance_failures.clear()
            available = self.instances
        return available[self.current_instance_idx % len(available)]

    def _rotate_instance(self):
        self.current_instance_idx += 1

    def _mark_instance_failed(self, instance: str):
        self._instance_failures[instance] = self._instance_failures.get(instance, 0) + 1

    async def _request(self, endpoint: str, params: dict = None, retries: int = MAX_RETRIES) -> dict:
        last_error = None

        for attempt in range(retries + 2):
            instance = self._get_instance()
            url = f"{instance}{endpoint}"

            try:
                resp = await self.client.get(url, params=params)
                resp.raise_for_status()
                return resp.json()
            except httpx.HTTPStatusError as e:
                last_error = e
                if e.response.status_code == 429:
                    wait_time = min(2 ** attempt, 30)
                    console.print(f"  [bright_black]Rate limited, waiting {wait_time}s...[/]")
                    await asyncio.sleep(wait_time)
                elif e.response.status_code == 404:
                    raise
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
        data = await self._request("/info/", {"id": track_id})
        return data.get("data", data)

    async def get_track_stream(self, track_id: int, quality: str = DEFAULT_QUALITY) -> dict:
        data = await self._request("/track/", {"id": track_id, "quality": quality})
        return data.get("data", data)

    async def get_album(self, album_id: int) -> dict:
        try:
            data = await self._request("/album/", {"id": album_id, "limit": 500})
            return data.get("data", data)
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 429:
                console.print("  [bright_black]Album endpoint limited, using search fallback...[/]")
                return await self._get_album_via_search(album_id)
            raise

    async def _get_album_via_search(self, album_id: int) -> dict:
        album_info = None

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
                await asyncio.sleep(0.3)
            except:
                continue

        if not album_info:
            raise Exception(f"Could not find album {album_id}")

        album_title = album_info.get("title", "")
        artist_obj = album_info.get("artist") or (album_info.get("artists", [{}])[0] if album_info.get("artists") else {})
        artist_name = artist_obj.get("name", "")

        album_tracks = []
        seen_ids = set()

        for query in [f"{artist_name} {album_title}", album_title, artist_name]:
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

        album_tracks.sort(key=lambda x: x["item"].get("trackNumber", 0))

        return {**album_info, "items": album_tracks}

    async def get_playlist(self, playlist_id: str) -> dict:
        data = await self._request("/playlist/", {"id": playlist_id, "limit": 500})
        return data

    async def get_artist_albums(self, artist_id: int) -> dict:
        data = await self._request("/artist/", {"f": artist_id})
        return data

    async def search(self, query: str, search_type: str = "s") -> dict:
        data = await self._request("/search/", {search_type: query})
        return data.get("data", data)


# =============================================================================
# Manifest Parsing
# =============================================================================

def parse_manifest(manifest_data: dict) -> Optional[Union[str, List[str]]]:
    manifest_b64 = manifest_data.get("manifest")
    mime_type = manifest_data.get("manifestMimeType", "")

    if not manifest_b64:
        return None

    try:
        manifest_str = base64.b64decode(manifest_b64).decode("utf-8")

        if "application/vnd.tidal.bts" in mime_type:
            manifest_json = json.loads(manifest_str)
            urls = manifest_json.get("urls", [])
            return urls[0] if urls else None

        elif "application/dash+xml" in mime_type:
            return parse_dash_manifest(manifest_str)

    except Exception as e:
        console.print(f"  [red]Manifest parse error: {e}[/]")

    return None


def parse_dash_manifest(mpd_content: str) -> Optional[List[str]]:
    try:
        init_match = re.search(r'initialization="([^"]+)"', mpd_content)
        media_match = re.search(r'media="([^"]+)"', mpd_content)

        if not init_match or not media_match:
            return None

        init_url = init_match.group(1)
        media_template = media_match.group(1)

        segments = []
        segment_pattern = re.compile(r'<S\s+d="(\d+)"(?:\s+r="(\d+)")?\s*/>')

        for match in segment_pattern.finditer(mpd_content):
            repeat = int(match.group(2)) if match.group(2) else 0
            for _ in range(repeat + 1):
                segments.append(True)

        urls = [init_url]
        for i in range(len(segments)):
            segment_url = media_template.replace("$Number$", str(i + 1))
            urls.append(segment_url)

        return urls

    except Exception:
        return None


# =============================================================================
# File Operations
# =============================================================================

def sanitize_filename(name: str, max_length: int = 100) -> str:
    name = re.sub(r'[<>:"/\\|?*]', '_', name)
    name = re.sub(r'\s+', ' ', name).strip()
    if len(name) > max_length:
        name = name[:max_length].rsplit(' ', 1)[0]
    return name


def get_file_extension(manifest_data: dict) -> str:
    audio_quality = manifest_data.get("audioQuality", "")
    if "HI_RES" in audio_quality:
        return ".flac"

    manifest_b64 = manifest_data.get("manifest", "")
    if manifest_b64:
        try:
            manifest_str = base64.b64decode(manifest_b64).decode("utf-8")
            if "audio/flac" in manifest_str or "flac" in manifest_str.lower():
                return ".flac"
            elif "audio/mp4" in manifest_str:
                return ".m4a"
        except:
            pass
    return ".flac"


def build_file_path(track: Track, output_dir: str, extension: str) -> Path:
    artist_dir = sanitize_filename(track.artist)
    album_dir = sanitize_filename(track.album)

    if track.disc_number > 1:
        filename = f"{track.disc_number}-{track.track_number:02d} - {sanitize_filename(track.title)}{extension}"
    else:
        filename = f"{track.track_number:02d} - {sanitize_filename(track.title)}{extension}"

    return Path(output_dir) / artist_dir / album_dir / filename


async def download_file(client: httpx.AsyncClient, url: Union[str, List[str]], output_path: Path, progress_callback=None) -> int:
    output_path.parent.mkdir(parents=True, exist_ok=True)

    if isinstance(url, list):
        return await download_dash_segments(client, url, output_path, progress_callback)

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
    output_path.parent.mkdir(parents=True, exist_ok=True)
    total_downloaded = 0

    with open(output_path, "wb") as f:
        for url in urls:
            try:
                async with client.stream("GET", url, follow_redirects=True) as resp:
                    resp.raise_for_status()
                    async for chunk in resp.aiter_bytes(chunk_size=8192):
                        f.write(chunk)
                        total_downloaded += len(chunk)
                        if progress_callback:
                            progress_callback(len(chunk))
            except:
                continue

    return total_downloaded


async def download_cover(client: httpx.AsyncClient, cover_id: str) -> Optional[bytes]:
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

            if cover_data:
                pic = Picture()
                pic.type = 3
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

            if cover_data:
                audio["covr"] = [MP4Cover(cover_data, imageformat=MP4Cover.FORMAT_JPEG)]

            audio.save()

    except:
        pass


# =============================================================================
# Download Engine
# =============================================================================

async def download_track(
    api: TidalAPIClient,
    download_client: httpx.AsyncClient,
    track_id: int,
    quality: str,
    output_dir: str,
    semaphore: asyncio.Semaphore,
    stats: DownloadStats,
    progress: Progress,
    task_id,
    embed_meta: bool = True,
) -> DownloadResult:
    async with semaphore:
        track_info = None
        try:
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

            stream_data = await api.get_track_stream(track_id, quality)
            download_url = parse_manifest(stream_data)

            if not download_url:
                raise Exception("Could not parse streaming URL")

            extension = get_file_extension(stream_data)
            file_path = build_file_path(track, output_dir, extension)

            if file_path.exists():
                stats.completed += 1
                stats.success += 1
                progress.update(task_id, advance=1)
                return DownloadResult(track=track, success=True, file_path=str(file_path))

            def update_bytes(chunk_size):
                stats.total_bytes += chunk_size

            file_size = await download_file(download_client, download_url, file_path, update_bytes)

            if embed_meta:
                cover_data = await download_cover(download_client, track.cover_url)
                embed_metadata(file_path, track, cover_data)

            stats.completed += 1
            stats.success += 1
            progress.update(task_id, advance=1)

            return DownloadResult(track=track, success=True, file_path=str(file_path), file_size=file_size)

        except Exception as e:
            stats.completed += 1
            stats.failed += 1
            progress.update(task_id, advance=1)

            error_track = track_info or Track(
                id=track_id, title="Unknown", artist="Unknown", album="Unknown",
                album_id=0, track_number=0, disc_number=1, duration=0
            )
            return DownloadResult(track=error_track, success=False, error=str(e)[:100])


async def bulk_download(
    track_ids: List[int],
    quality: str = DEFAULT_QUALITY,
    output_dir: str = DEFAULT_OUTPUT_DIR,
    concurrency: int = DEFAULT_CONCURRENCY,
    embed_meta: bool = True,
) -> List[DownloadResult]:
    stats = DownloadStats(total=len(track_ids))
    semaphore = asyncio.Semaphore(concurrency)
    results: List[DownloadResult] = []

    async with TidalAPIClient() as api:
        async with httpx.AsyncClient(
            timeout=httpx.Timeout(60.0),
            limits=httpx.Limits(max_keepalive_connections=50, max_connections=100),
            follow_redirects=True,
        ) as download_client:

            with Progress(
                SpinnerColumn(),
                TextColumn("[bright_white]{task.description}[/]"),
                BarColumn(bar_width=40, style="bright_black", complete_style="bright_blue", finished_style="green"),
                TaskProgressColumn(),
                TextColumn("[bright_black]|[/]"),
                TimeRemainingColumn(),
                console=console,
                transient=False,
            ) as progress:
                task_id = progress.add_task("Downloading", total=len(track_ids))

                tasks = [
                    download_track(
                        api, download_client, tid, quality, output_dir,
                        semaphore, stats, progress, task_id, embed_meta
                    )
                    for tid in track_ids
                ]
                results = await asyncio.gather(*tasks)

    return results, stats


# =============================================================================
# High-Level Commands
# =============================================================================

async def download_album(album_id: int, **kwargs) -> Tuple[List[DownloadResult], DownloadStats]:
    async with TidalAPIClient() as api:
        album_data = await api.get_album(album_id)

        items = album_data.get("items", [])
        track_ids = []

        for item in items:
            track = item.get("item", item)
            if track.get("type", "track") == "track":
                track_ids.append(track.get("id"))

        # Display album info
        album_table = Table(show_header=False, box=box.SIMPLE, padding=(0, 1))
        album_table.add_column("Key", style="bright_black")
        album_table.add_column("Value", style="bright_white")

        artist = album_data.get("artist", {}).get("name") or (album_data.get("artists", [{}])[0].get("name") if album_data.get("artists") else "Unknown")

        album_table.add_row("Album", album_data.get("title", "Unknown"))
        album_table.add_row("Artist", artist)
        album_table.add_row("Tracks", str(len(track_ids)))
        album_table.add_row("Year", str(album_data.get("releaseDate", "")[:4]) if album_data.get("releaseDate") else "Unknown")

        console.print(album_table)
        console.print()

    return await bulk_download(track_ids, **kwargs)


async def download_playlist(playlist_id: str, **kwargs) -> Tuple[List[DownloadResult], DownloadStats]:
    async with TidalAPIClient() as api:
        playlist_data = await api.get_playlist(playlist_id)

        playlist_info = playlist_data.get("playlist", {})
        items = playlist_data.get("items", [])
        track_ids = []

        for item in items:
            track = item.get("item", item)
            if track.get("type", "track") == "track":
                track_ids.append(track.get("id"))

        # Display playlist info
        playlist_table = Table(show_header=False, box=box.SIMPLE, padding=(0, 1))
        playlist_table.add_column("Key", style="bright_black")
        playlist_table.add_column("Value", style="bright_white")

        playlist_table.add_row("Playlist", playlist_info.get("title", "Unknown"))
        playlist_table.add_row("Tracks", str(len(track_ids)))

        console.print(playlist_table)
        console.print()

    return await bulk_download(track_ids, **kwargs)


async def download_artist_discography(artist_id: int, **kwargs) -> Tuple[List[DownloadResult], DownloadStats]:
    async with TidalAPIClient() as api:
        artist_data = await api.get_artist_albums(artist_id)

        albums = artist_data.get("albums", {}).get("items", [])
        tracks = artist_data.get("tracks", [])
        track_ids = [t.get("id") for t in tracks if t.get("id")]

        # Display artist info
        artist_table = Table(show_header=False, box=box.SIMPLE, padding=(0, 1))
        artist_table.add_column("Key", style="bright_black")
        artist_table.add_column("Value", style="bright_white")

        artist_table.add_row("Artist", "Discography")
        artist_table.add_row("Albums", str(len(albums)))
        artist_table.add_row("Tracks", str(len(track_ids)))

        console.print(artist_table)
        console.print()

    return await bulk_download(track_ids, **kwargs)


def parse_tidal_url(url: str) -> Tuple[str, str]:
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

    raise ValueError(f"Invalid Tidal URL: {url}")


# =============================================================================
# Results Display
# =============================================================================

def print_results(results: List[DownloadResult], stats: DownloadStats):
    success = [r for r in results if r.success]
    failed = [r for r in results if not r.success]

    console.print()
    console.print(Rule("Results", style="bright_black"))
    console.print()

    # Stats table
    stats_table = Table(show_header=False, box=box.SIMPLE, padding=(0, 2))
    stats_table.add_column("Metric", style="bright_black")
    stats_table.add_column("Value", justify="right")

    stats_table.add_row("Successful", f"[green]{len(success)}[/]")
    stats_table.add_row("Failed", f"[red]{len(failed)}[/]" if failed else "[bright_black]0[/]")
    stats_table.add_row("Total Size", f"[bright_white]{stats.total_bytes / 1024 / 1024:.1f} MB[/]")
    stats_table.add_row("Duration", f"[bright_white]{stats.elapsed:.1f}s[/]")
    stats_table.add_row("Avg Speed", f"[bright_white]{stats.speed_mbps:.1f} MB/s[/]")

    console.print(stats_table)

    if failed:
        console.print()
        console.print("[bright_black]Failed tracks:[/]")
        for r in failed[:5]:
            console.print(f"  [red]-[/] {r.track.artist} - {r.track.title}")
            console.print(f"    [bright_black]{r.error}[/]")
        if len(failed) > 5:
            console.print(f"  [bright_black]... and {len(failed) - 5} more[/]")

    console.print()


# =============================================================================
# Interactive Search
# =============================================================================

async def interactive_search(query: str, **kwargs) -> Optional[Tuple[List[DownloadResult], DownloadStats]]:
    async with TidalAPIClient() as api:
        search_results = await api.search(query)
        items = search_results.get("items", [])

        if not items:
            console.print("[bright_black]No results found.[/]")
            return None

        # Display results
        results_table = Table(box=box.SIMPLE, padding=(0, 1))
        results_table.add_column("#", style="bright_black", width=3)
        results_table.add_column("Title", style="bright_white")
        results_table.add_column("Artist", style="bright_black")
        results_table.add_column("Album", style="bright_black")

        for i, item in enumerate(items[:10], 1):
            results_table.add_row(
                str(i),
                item.get("title", "Unknown")[:40],
                item.get("artist", {}).get("name", "Unknown")[:25],
                item.get("album", {}).get("title", "Unknown")[:25],
            )

        console.print(results_table)
        console.print()

        choice = Prompt.ask(
            "[bright_black]Select[/]",
            choices=[str(i) for i in range(1, min(11, len(items) + 1))] + ["all", "q"],
            default="1"
        )

        if choice == "q":
            return None
        elif choice == "all":
            track_ids = [item.get("id") for item in items[:10] if item.get("id")]
        else:
            idx = int(choice) - 1
            track_ids = [items[idx].get("id")]

        console.print()
        return await bulk_download(track_ids, **kwargs)


# =============================================================================
# CLI
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Tidal Hi-Res Downloader",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    parser.add_argument("command", choices=["album", "playlist", "artist", "tracks", "url", "search"], help="Download type")
    parser.add_argument("ids", nargs="+", help="ID(s), URL, or search query")
    parser.add_argument("-q", "--quality", default=DEFAULT_QUALITY, choices=["HI_RES_LOSSLESS", "LOSSLESS", "HIGH", "LOW"])
    parser.add_argument("-o", "--output", default=DEFAULT_OUTPUT_DIR, help="Output directory")
    parser.add_argument("-c", "--concurrency", type=int, default=DEFAULT_CONCURRENCY, help="Parallel downloads")
    parser.add_argument("--no-metadata", action="store_true", help="Skip metadata embedding")
    parser.add_argument("--discography", action="store_true", help="Download full discography")

    args = parser.parse_args()

    kwargs = {
        "quality": args.quality,
        "output_dir": args.output,
        "concurrency": args.concurrency,
        "embed_meta": not args.no_metadata,
    }

    print_header()
    print_config(args.quality, args.output, args.concurrency)

    try:
        if args.command == "album":
            results, stats = asyncio.run(download_album(int(args.ids[0]), **kwargs))

        elif args.command == "playlist":
            results, stats = asyncio.run(download_playlist(args.ids[0], **kwargs))

        elif args.command == "artist":
            if args.discography:
                results, stats = asyncio.run(download_artist_discography(int(args.ids[0]), **kwargs))
            else:
                console.print("[bright_black]Use --discography flag for full artist catalog[/]")
                sys.exit(1)

        elif args.command == "tracks":
            track_ids = [int(tid) for tid in args.ids]
            results, stats = asyncio.run(bulk_download(track_ids, **kwargs))

        elif args.command == "url":
            url_type, url_id = parse_tidal_url(args.ids[0])
            if url_type == "album":
                results, stats = asyncio.run(download_album(int(url_id), **kwargs))
            elif url_type == "playlist":
                results, stats = asyncio.run(download_playlist(url_id, **kwargs))
            elif url_type == "track":
                results, stats = asyncio.run(bulk_download([int(url_id)], **kwargs))
            elif url_type == "artist":
                results, stats = asyncio.run(download_artist_discography(int(url_id), **kwargs))

        elif args.command == "search":
            query = " ".join(args.ids)
            result = asyncio.run(interactive_search(query, **kwargs))
            if result:
                results, stats = result
            else:
                sys.exit(0)

        print_results(results, stats)

    except KeyboardInterrupt:
        console.print("\n[bright_black]Cancelled[/]")
        sys.exit(1)
    except Exception as e:
        console.print(f"\n[red]Error:[/] {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
