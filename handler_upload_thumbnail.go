package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse thumbnail", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve file", err)
		return
	}

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse media type", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User does not own video", err)
		return
	}

	extension, err := convertMediaTypeToExtension(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't convert media type to extension", err)
		return
	}

	randomPath := make([]byte, 32)
	_, err = rand.Read(randomPath)
	encodedPath := base64.URLEncoding.EncodeToString(randomPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create random number", err)
		return
	}
	filePath := filepath.Join("./assets/", fmt.Sprintf("%s%s", encodedPath, extension))
	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file on disk", err)
		return
	}
	
	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file to disk", err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s%s", cfg.port, encodedPath, extension)
	
	updatedVideo := database.Video{
		ID: videoID,
		CreatedAt: video.CreatedAt,
		UpdatedAt: time.Now(),
		ThumbnailURL: &thumbnailURL, 
		VideoURL: video.VideoURL,     
		CreateVideoParams: database.CreateVideoParams{
			Title: video.Title,
			Description: video.Description,
			UserID: userID,
		},
	}
	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, updatedVideo)
}

func convertMediaTypeToExtension(mediaType string) (string, error) {
	if mediaType == "image/png"{
		return ".png", nil
	} else if mediaType == "image/jpeg"{
		return ".jpg", nil
	}
	return "", fmt.Errorf("invalid media type")
}
