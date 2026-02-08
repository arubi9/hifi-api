package download

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	goflac "github.com/go-flac/go-flac"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
)

const remuxTimeout = 60 * time.Second

// Remux converts an M4A container to FLAC using ffmpeg stream copy.
// Returns the final output path.
func Remux(ctx context.Context, inputPath, outputPath string) error {
	ctx, cancel := context.WithTimeout(ctx, remuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", inputPath,
		"-c:a", "copy",
		"-vn",
		"-y",
		outputPath,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		// Fallback: rename tmp to .m4a
		fallback := inputPath[:len(inputPath)-4] // strip .tmp
		os.Rename(inputPath, fallback)
		return fmt.Errorf("ffmpeg remux: %w (kept as %s)", err, filepath.Base(fallback))
	}

	// Remove temp file
	os.Remove(inputPath)
	return nil
}

// EmbedMetadata writes metadata tags to a FLAC or M4A file.
// For FLAC: uses native go-flac library for Vorbis comments + picture.
// For M4A: uses ffmpeg to write metadata.
func EmbedMetadata(ctx context.Context, filePath string, track models.Track, coverData []byte) error {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".flac":
		return embedFLACMetadata(filePath, track, coverData)
	case ".m4a":
		return embedM4AMetadata(ctx, filePath, track, coverData)
	default:
		return nil
	}
}

// embedFLACMetadata writes Vorbis comments and cover art to a FLAC file natively.
func embedFLACMetadata(filePath string, track models.Track, coverData []byte) error {
	f, err := goflac.ParseFile(filePath)
	if err != nil {
		logger.Log.Warn("failed to parse FLAC for metadata", "path", filePath, "err", err)
		return fmt.Errorf("parse FLAC: %w", err)
	}

	// Build Vorbis comment block
	cmts := flacvorbis.New()
	cmts.Add(flacvorbis.FIELD_TITLE, track.Title)
	cmts.Add(flacvorbis.FIELD_ARTIST, track.Artist)
	cmts.Add(flacvorbis.FIELD_ALBUM, track.Album)
	if track.TrackNumber > 0 {
		cmts.Add(flacvorbis.FIELD_TRACKNUMBER, strconv.Itoa(track.TrackNumber))
	}
	if track.Year > 0 {
		cmts.Add(flacvorbis.FIELD_DATE, strconv.Itoa(track.Year))
	}

	// Replace or add Vorbis comment block
	cmtBlock := cmts.Marshal()
	replaced := false
	for i, block := range f.Meta {
		if block.Type == goflac.VorbisComment {
			f.Meta[i] = &cmtBlock
			replaced = true
			break
		}
	}
	if !replaced {
		f.Meta = append(f.Meta, &cmtBlock)
	}

	// Embed cover art
	if len(coverData) > 0 {
		pic, err := flacpicture.NewFromImageData(
			flacpicture.PictureTypeFrontCover,
			"Front Cover",
			coverData,
			"image/jpeg",
		)
		if err == nil {
			picBlock := pic.Marshal()
			// Remove existing picture blocks
			filtered := make([]*goflac.MetaDataBlock, 0, len(f.Meta))
			for _, block := range f.Meta {
				if block.Type != goflac.Picture {
					filtered = append(filtered, block)
				}
			}
			f.Meta = filtered
			f.Meta = append(f.Meta, &picBlock)
		} else {
			logger.Log.Debug("failed to create FLAC picture", "err", err)
		}
	}

	return f.Save(filePath)
}

// embedM4AMetadata uses ffmpeg to write metadata to M4A files.
func embedM4AMetadata(ctx context.Context, filePath string, track models.Track, coverData []byte) error {
	tmpOut := filePath + ".meta.tmp"
	args := []string{
		"-i", filePath,
	}

	// Add cover art if available
	var coverFile string
	if len(coverData) > 0 {
		coverFile = filePath + ".cover.jpg"
		if err := os.WriteFile(coverFile, coverData, 0644); err == nil {
			args = append(args, "-i", coverFile)
			defer os.Remove(coverFile)
		}
	}

	args = append(args, "-c", "copy")

	// Add metadata
	args = append(args,
		"-metadata", fmt.Sprintf("title=%s", track.Title),
		"-metadata", fmt.Sprintf("artist=%s", track.Artist),
		"-metadata", fmt.Sprintf("album=%s", track.Album),
	)
	if track.TrackNumber > 0 {
		args = append(args, "-metadata", fmt.Sprintf("track=%d", track.TrackNumber))
	}
	if track.Year > 0 {
		args = append(args, "-metadata", fmt.Sprintf("date=%d", track.Year))
	}

	// Map cover art if provided
	if coverFile != "" {
		args = append(args, "-map", "0:a", "-map", "1:v",
			"-disposition:v:0", "attached_pic")
	}

	args = append(args, "-y", tmpOut)

	ctx, cancel := context.WithTimeout(ctx, remuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		os.Remove(tmpOut)
		logger.Log.Debug("ffmpeg metadata embed failed", "err", err)
		return nil // Non-fatal: file still playable without metadata
	}

	// Replace original with tagged version
	os.Remove(filePath)
	return os.Rename(tmpOut, filePath)
}
