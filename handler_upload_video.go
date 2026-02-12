package main

import (

	"fmt"
	"path"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

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


	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}

	if video.UserID != userID{
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	file, handler, err := r.FormFile("video") 
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem with FormFile 'video'", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(handler.Header.Get("Content-Type"))

	if (mediaType != "video/mp4"){
		respondWithError(w, http.StatusBadRequest, "must be mp4", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "problem with CreateTemp", err)
		return	
	}

	defer tempFile.Close()
	defer os.Remove(tempFile.Name())


	if _, err = io.Copy(tempFile, file); err != nil{
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return		
	}

	if _, err = tempFile.Seek(0, io.SeekStart); err != nil{
		respondWithError(w, http.StatusInternalServerError, "Error resetting ptr", err)
		return
	}

	aspect, err := getVideoAspectRatio(tempFile.Name())
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Error getting aspect ratio", err)
		return
	}

	var aspectPrefix string
	switch (aspect){
	case "16:9":
		aspectPrefix = "landscape"
	case "9:16":
		aspectPrefix = "portrait"
	default:
		aspectPrefix = "other"
	}

	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing file path for fast start", err)
		return
	}
	defer os.Remove(processedFilePath)

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed file", err)
		return
	}
	defer processedFile.Close()

	key := path.Join(aspectPrefix , getAssetPath(mediaType))
	bucketKey := fmt.Sprintf("%s,%s", cfg.s3Bucket, key)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: 	 aws.String(cfg.s3Bucket),
		Key:		 aws.String(key),
		Body:		 processedFile,
		ContentType: aws.String(mediaType),
	})

	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}
	//url := cfg.getObjectURL(key)
	url := bucketKey
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)

}
