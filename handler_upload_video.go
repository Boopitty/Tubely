package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Get the video ID from the URL path and validate it.
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Get the token from the authorization header and validate it.
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validate the JWT and get the user ID from it.
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	// This is the maximum size of the video file. Adjust as needed.
	maxMemory := int64(1 << 30) // 1GB
	closer := http.MaxBytesReader(w, r.Body, maxMemory)
	defer closer.Close()

	// get video metadata from database and check if the user is the owner
	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}
	if dbVideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have permission to upload this video", nil)
		return
	}

	// "video" is the key of the form field that contains the file. Adjust as needed.
	vidFile, vidHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to get video file", err)
		return
	}
	defer vidFile.Close()

	// validate the media type.
	mediaType := vidHeader.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type, only video/mp4 is allowed", nil)
		return
	}

	// Create temporary file for the video.
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy the uploaded video file to the temporary file.
	_, err = io.Copy(tempFile, vidFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy video file", err)
		return
	}
	tempFile.Seek(0, io.SeekStart) // reset file pointer to the beginning for later reading

	// Process the video file for fast start streaming.
	// The proccessed video will be uploaded to s3 and the original video file will be deleted after processing.
	outputFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}
	defer os.Remove(outputFilePath)
	outputFile, err := os.Open(outputFilePath) // open the new file for use
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not open output file", err)
		return
	}
	defer outputFile.Close()

	// Get the aspect ratio of the video file.
	aspectRatio, err := getVideoAspectRatio(outputFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}
	switch aspectRatio {
	case "16:9":
		aspectRatio = "landscape"
	case "9:16":
		aspectRatio = "portrait"
	default:
		aspectRatio = "other"
	}

	// Generate a random file name for the video file in S3.
	bytesKey := make([]byte, 32)
	_, err = io.ReadFull(rand.Reader, bytesKey)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random file name", err)
		return
	}
	stringKey := base64.URLEncoding.EncodeToString(bytesKey)                             // e.g. "randomstring"
	fileKey := fmt.Sprintf("%s/%s.%s", aspectRatio, stringKey, mimeType[len("video/"):]) // e.g. "randomstring.mp4"

	// Upload the video file to S3.
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        outputFile,
		ContentType: &mimeType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video file", err)
		return
	}

	// Update the video record in the database with the new video URL.
	bucketKey := fmt.Sprintf("%s,%s", cfg.s3Bucket, fileKey)
	dbVideo.VideoURL = &bucketKey
	signedVideo, err := cfg.dbVideoToSignedVideo(dbVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't sign video", err)
		return
	}
	err = cfg.db.UpdateVideo(signedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}
}
