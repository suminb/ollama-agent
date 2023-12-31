package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

func downloadPrompt(s3Url string) []byte {
	ctx := context.Background()
	sdkConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Println("Couldn't load default configuration. Have you set up your AWS account?")
		fmt.Println(err)
		return nil
	}
	s3Client := s3.NewFromConfig(sdkConfig)

	u, _ := url.Parse(s3Url)
	fmt.Printf("host: %s, path: %s\n", u.Host, u.Path)

	s3ObjectOutput, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.Host),
		Key:    aws.String(u.Path[1:]),
	})
	if err != nil {
		panic(err)
	}

	s3ObjectBytes, err := ioutil.ReadAll(s3ObjectOutput.Body)
	if err != nil {
		panic(err)
	}

	return s3ObjectBytes
}

func main() {
	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	r.POST("/query", func(c *gin.Context) {
		err := c.Request.ParseForm()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"error": err.Error(),
			})
			return
		}
		promptUrl := c.Request.Form.Get("promptUrl")

		promptString := downloadPrompt(promptUrl)

		out, err := exec.Command("ollama", "run", "dolphin-mixtral", string(promptString)).Output()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": string(out),
		})
	})
	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
