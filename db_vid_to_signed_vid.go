package main

import (
	"strings"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

// take a video database.Video as input and return a database.Video with the VideoURL field set to a presigned URL and an error
func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	bucket_key := strings.Split(*video.VideoURL, ",")
	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket_key[0], bucket_key[1], time.Duration(time.Duration(3).Hours()))
	if err != nil {
		return database.Video{}, nil
	}
	video.VideoURL = &presignedURL
	return video, nil
}
