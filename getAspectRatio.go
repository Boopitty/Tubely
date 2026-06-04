package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Find the aspect ratio of a video file using ffprobe.
func getVideoAspectRatio(filePath string) (string, error) {
	// Command to run ffprobe to get the stream info in JSON format
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	// Run the command and capture the output
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
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
