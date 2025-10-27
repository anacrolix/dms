package dms

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/anacrolix/ffprobe"
)

// generateThumbnailFFmpeg generates a thumbnail using ffmpeg instead of ffmpegthumbnailer.
// This provides better cross-platform compatibility, especially on Windows.
func generateThumbnailFFmpeg(ctx context.Context, inputPath string) ([]byte, error) {
	// Get video duration to calculate the seek position
	ss, err := getThumbnailSeekPosition(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("determining seek position: %w", err)
	}

	// Build ffmpeg arguments for thumbnail generation
	args := []string{
		"-xerror",
		"-loglevel", "warning",
		"-ss", ss,
		"-i", inputPath,
		"-vf", "thumbnail",
		"-frames:v", "1",
		"-f", "image2",
		"-c:v", "png",
		"pipe:",
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	// Start the command
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}

	// Set up context cancellation
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Read the thumbnail data
	thumbnailData, err := io.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("reading thumbnail data: %w", err)
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("ffmpeg command failed: %w", err)
	}

	return thumbnailData, nil
}

// getThumbnailSeekPosition calculates the seek position for thumbnail extraction.
// It seeks to 1/4 of the video duration by default.
func getThumbnailSeekPosition(ctx context.Context, inputPath string) (string, error) {
	// Use ffprobe to get video duration
	pc, err := ffprobe.Start(inputPath)
	if err != nil {
		// If ffprobe fails, default to 10 seconds
		return "10", nil
	}

	select {
	case <-ctx.Done():
		if pc.Cmd.Process != nil {
			pc.Cmd.Process.Kill()
		}
		return "", ctx.Err()
	case <-pc.Done:
	}

	if pc.Err != nil {
		// If ffprobe fails, default to 10 seconds
		return "10", nil
	}

	duration, err := pc.Info.Duration()
	if err != nil {
		// If duration cannot be determined, default to 10 seconds
		return "10", nil
	}

	// Seek to 1/4 of the duration
	seekDuration := duration / 4
	return strconv.FormatFloat(seekDuration.Seconds(), 'f', -1, 64), nil
}
