package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// Parse the for data

	// This is the maximum MB size of the form data, including the file. Adjust as needed.
	maxMemory := int64(10 << 20) // 10 MB

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form data", err)
		return
	}

	// "thumbnail" is the key of the form field that contains the file. Adjust as needed.
	fileData, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't read thumbnail file", err)
		return
	}
	defer fileData.Close()

	// The media type of the uploaded file from the Content-Type header
	mediaType := fileHeader.Header.Get("Content-Type")

	// Parse and validate the media type.
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type. Only JPEG and PNG are allowed.", nil)
		return
	}

	// The actual image file data as a byte slice
	imageData, err := io.ReadAll(fileData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read thumbnail file data", err)
		return
	}

	// Get the video from the db, and check if the user is the correct owner.
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You are not the owner of this video", nil)
		return
	}

	// Generate random file name for the thumbnail.
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random bytes", err)
		return
	}
	encoded := base64.URLEncoding.EncodeToString(randomBytes)
	// thumbnail path: /assets/<videoID>.<file_extension>
	thumbnailPath := fmt.Sprintf("%s/%s.%s", cfg.assetsRoot, encoded, mimeType[len("image/"):])

	// Create a file in the assets directory
	assetFile, err := os.Create(thumbnailPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create thumbnail file", err)
		return
	}
	defer assetFile.Close()

	// Write the image data to the new file
	_, err = io.Copy(assetFile, bytes.NewReader(imageData))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save thumbnail file", err)
		return
	}

	// Update the video record in the database with the thumbnail URL
	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, encoded, mediaType[len("image/"):])
	video.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video thumbnail in database", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
