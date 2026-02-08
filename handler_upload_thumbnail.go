package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	//"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

	const maxMemory = 10 << 20;
	r.ParseMultipartForm(maxMemory);

	file, header, err := r.FormFile("thumbnail");
	if err != nil{
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err);
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type");
	imgs, err := io.ReadAll(file);
	if err != nil{
		respondWithError(w, http.StatusInternalServerError, "Unable to read image daata", err)
		return
	}

	vid, err := cfg.db.GetVideo(videoID);
	if err != nil{
		respondWithError(w, http.StatusNotFound, "Video not found in db", err)
		return
	}

	if vid.UserID != userID{
		respondWithError(w, http.StatusUnauthorized, "User is not the video owner", err)
		return
	}

	newThumbnail := thumbnail{
		data: imgs,
		mediaType: mediaType,
	}

	videoThumbnails[videoID] = newThumbnail;

	formattedURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID);
	vid.ThumbnailURL = &formattedURL
	err = cfg.db.UpdateVideo(vid);
	if err != nil{
		delete(videoThumbnails, videoID)
		respondWithError(w, http.StatusInternalServerError, "unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, vid);
}
