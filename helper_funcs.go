package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Find the aspect ratio of a video file using ffprobe.
func getVideoAspectRatio(filePath string) (string, error) {
	// Command to run ffprobe to get the stream info in JSON format
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	// Run the command
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	// Capture output
	cmdOutput := struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}{}
	json.Unmarshal(out.Bytes(), &cmdOutput)

	// Compute the aspect ratio from the width and height.
	width := float64(cmdOutput.Streams[0].Width)
	height := float64(cmdOutput.Streams[0].Height)

	if width > height {
		ratio := width / height
		if ratio <= 1.78 && ratio >= 1.76 {
			return "16:9", nil
		}
	} else {
		ratio := height / width
		if ratio <= 1.78 && ratio >= 1.76 {
			return "9:16", nil
		}
	}

	stringRatio := fmt.Sprintf("%d:%d", int(width), int(height))
	return stringRatio, nil
}

// Takes a file path as input and creates and returns a new path to a file with "fast start" encoding.
func processVideoForFastStart(filePath string) (string, error) {
	outFilePath := fmt.Sprintf("%s.%s", filePath, "processing")
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outFilePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w: %s", err, stderr.String())
	}
	return outFilePath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	client := s3.NewPresignClient(s3Client)
	input := s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	object, err := client.PresignGetObject(context.Background(), &input, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return object.URL, nil
}
