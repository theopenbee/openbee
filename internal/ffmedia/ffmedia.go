package ffmedia

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// AudioDurationMs returns the audio duration in milliseconds via ffprobe.
// Returns 0 and logs a warning if ffprobe is unavailable or fails.
func AudioDurationMs(ctx context.Context, path, ffprobePath string) (int, error) {
	sec, err := probeDuration(ctx, path, ffprobePath)
	if err != nil {
		return 0, err
	}
	return int(sec * 1000), nil
}

// VideoDurationSec returns the video duration in whole seconds via ffprobe.
// Returns 0 and logs a warning if ffprobe is unavailable or fails.
func VideoDurationSec(ctx context.Context, path, ffprobePath string) (int, error) {
	sec, err := probeDuration(ctx, path, ffprobePath)
	if err != nil {
		return 0, err
	}
	return int(sec), nil
}

// ExtractFirstFrame extracts the first video frame to a temp JPEG file.
// Returns the temp file path and a cleanup func. Caller must call cleanup().
// Returns an error if ffmpeg is unavailable or fails.
func ExtractFirstFrame(ctx context.Context, videoPath, ffmpegPath string) (thumbPath string, cleanup func(), err error) {
	tmp, err := os.CreateTemp("", "openbee-thumb-*.jpg")
	if err != nil {
		return "", func() {}, err
	}
	tmp.Close()
	thumbPath = tmp.Name()

	cleanup = func() { os.Remove(thumbPath) }

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-i", videoPath,
		"-ss", "00:00:00",
		"-vframes", "1",
		"-y", thumbPath,
	)
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		var exitErr *exec.ExitError
		if errors.As(cmdErr, &exitErr) || errors.Is(cmdErr, exec.ErrNotFound) {
			slog.Warn("ffmpeg extract first frame failed", "component", "ffmedia", "error", cmdErr, "output", string(out))
		}
		cleanup()
		return "", func() {}, cmdErr
	}
	return thumbPath, cleanup, nil
}

// probeDuration runs ffprobe and returns the duration in fractional seconds.
func probeDuration(ctx context.Context, path, ffprobePath string) (float64, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) || errors.Is(err, exec.ErrNotFound) {
			slog.Warn("ffprobe failed", "component", "ffmedia", "error", err)
		}
		return 0, err
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, err
	}
	sec, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, err
	}
	return sec, nil
}
