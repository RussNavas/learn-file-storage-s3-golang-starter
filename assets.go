package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string{
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil{
		panic("failed to generate random bytes")
	}
	id := base64.RawURLEncoding.EncodeToString(base)

	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", id, ext)
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmdStruct := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buff []byte;
	outBuff := bytes.NewBuffer(buff)
	cmdStruct.Stdout = outBuff
	err := cmdStruct.Run()
	if err != nil{
		return "unable to run cmd", err
	}

	type VideoInfo struct {
		Streams [] struct {
			Width 	int 	`json:"width"`
			Height 	int 	`json:"height"`
		}`json:"streams"`
	}

	videoInfo := VideoInfo{}
	err = json.Unmarshal(outBuff.Bytes(), &videoInfo)
	if err != nil{
		return "unable to unmarshal", err
	}

	width := videoInfo.Streams[0].Width
    height := videoInfo.Streams[0].Height
    fmt.Printf("Width: %d, Height: %d\n", width, height)
	fmt.Printf("16*height/9 = %d, 16*width/9 = %d\n", 16*height/9, 16*width/9)
	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}

    return "other", nil
}

func processVideoForFastStart(filePath string) (string, error){
	outputFilePath := filePath+".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil{
		return "unable to run cmd", fmt.Errorf("ffmpeg failed: %s: %w", stderr.String(), err)
	}
	return outputFilePath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error){
	// 1. Use the SDK to create a s3.PresignClient
	presignClient := s3.NewPresignClient(s3Client)

	// 2. Use the client's .PresignGetObject() method
	// We pass s3.WithPresignExpires(expireTime) as the functional option at the end.
	req, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	// 3. Return the .URL field of the v4.PresignedHTTPRequest
	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error){
	if video.VideoURL == nil {
		return video, nil
	}

	splitString := strings.Split(*video.VideoURL, ",")
	if len(splitString) < 2 {
		return video, nil 
	}
	bucket := splitString[0]
	key := splitString[1]
	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, (15*time.Minute))
	if err != nil{
		return database.Video{}, err
	}

	video.VideoURL = &presignedURL

	return video, nil
}
