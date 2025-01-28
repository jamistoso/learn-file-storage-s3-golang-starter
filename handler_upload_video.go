package main

import (
	"crypto/rand"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

const maxMemory = 1 << 30

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

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

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse video", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve file", err)
		return
	}

	defer file.Close()

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

	extension, err := validateVideoMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't convert media type to extension", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary video file", err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file to disk", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset tempFile file pointer", err)
		return
	}

	randomPath := make([]byte, 32)
	_, err = rand.Read(randomPath)
	encodedPath := base64.URLEncoding.EncodeToString(randomPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create random number", err)
		return
	}

	objectKey := fmt.Sprintf("%v%v", encodedPath, extension)

	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: 			aws.String(cfg.s3Bucket),
		Key:    			aws.String(objectKey),
		Body:   			tempFile,
		ContentType:	&mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't put object into bucket", err)
		return
	}


	newVideoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, objectKey)
	updatedVideo := database.Video{
		ID: videoID,
		CreatedAt: video.CreatedAt,
		UpdatedAt: time.Now(),
		ThumbnailURL: video.ThumbnailURL, 
		VideoURL: &newVideoURL,     
		CreateVideoParams: database.CreateVideoParams{
			Title: video.Title,
			Description: video.Description,
			UserID: userID,
		},
	}
	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update database video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, updatedVideo)
}

func validateVideoMediaType(mediaType string) (string, error) {
	if mediaType == "video/mp4"{
		return "video/mp4", nil
	}
	return "", fmt.Errorf("invalid media type")
}