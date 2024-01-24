package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v57/github"
	"github.com/redis/go-redis/v9"
)

func handleError(ctx *gin.Context, err error) {
	ctx.JSON(http.StatusOK, gin.H{
		"error": err.Error(),
	})
}

func getS3ClientWithEndpoint(ctx *context.Context, endpoint string) (*s3.Client, error) {

	if endpoint == "" {
		sdkConfig, err := config.LoadDefaultConfig(*ctx)
		if err != nil {
			return nil, err
		}
		return s3.NewFromConfig(sdkConfig), nil
	} else {
		endpointResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
				Source:            aws.EndpointSourceCustom,
			}, nil
		})
		sdkConfig, err := config.LoadDefaultConfig(
			*ctx,
			config.WithEndpointResolverWithOptions(endpointResolver),
			config.WithRegion("auto"),
		)
		if err != nil {
			return nil, err
		}
		return s3.NewFromConfig(sdkConfig), nil
	}
}

func downloadPrompt(ctx *context.Context, s3Url string) ([]byte, error) {
	s3Endpoint := os.Getenv("AWS_S3_ENDPOINT")
	s3Client, err := getS3ClientWithEndpoint(ctx, s3Endpoint)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(s3Url)
	s3ObjectOutput, err := s3Client.GetObject(*ctx, &s3.GetObjectInput{
		Bucket: aws.String(u.Host),
		Key:    aws.String(u.Path[1:]),
	})
	if err != nil {
		return nil, err
	}

	s3ObjectBytes, err := ioutil.ReadAll(s3ObjectOutput.Body)
	if err != nil {
		return nil, err
	}

	return s3ObjectBytes, nil
}

// SQSReceiveMessageAPI defines the interface for the GetQueueUrl function.
// We use this interface to test the function using a mocked service.
type SQSReceiveMessageAPI interface {
	GetQueueUrl(ctx context.Context,
		params *sqs.GetQueueUrlInput,
		optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)

	ReceiveMessage(ctx context.Context,
		params *sqs.ReceiveMessageInput,
		optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
}

// SQSDeleteMessageAPI defines the interface for the GetQueueUrl and DeleteMessage functions.
// We use this interface to test the functions using a mocked service.
type SQSDeleteMessageAPI interface {
	GetQueueUrl(ctx context.Context,
		params *sqs.GetQueueUrlInput,
		optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)

	DeleteMessage(ctx context.Context,
		params *sqs.DeleteMessageInput,
		optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// GetQueueURL gets the URL of an Amazon SQS queue.
// Inputs:
//
//	c is the context of the method call, which includes the AWS Region.
//	api is the interface that defines the method call.
//	input defines the input arguments to the service call.
//
// Output:
//
//	If success, a GetQueueUrlOutput object containing the result of the service call and nil.
//	Otherwise, nil and an error from the call to GetQueueUrl.
func GetQueueURL(c context.Context, api SQSReceiveMessageAPI, input *sqs.GetQueueUrlInput) (*sqs.GetQueueUrlOutput, error) {
	return api.GetQueueUrl(c, input)
}

// GetMessages gets the most recent message from an Amazon SQS queue.
// Inputs:
//
//	c is the context of the method call, which includes the AWS Region.
//	api is the interface that defines the method call.
//	input defines the input arguments to the service call.
//
// Output:
//
//	If success, a ReceiveMessageOutput object containing the result of the service call and nil.
//	Otherwise, nil and an error from the call to ReceiveMessage.
func GetMessages(c context.Context, api SQSReceiveMessageAPI, input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
	return api.ReceiveMessage(c, input)
}

// RemoveMessage deletes a message from an Amazon SQS queue.
// Inputs:
//
//	c is the context of the method call, which includes the AWS Region.
//	api is the interface that defines the method call.
//	input defines the input arguments to the service call.
//
// Output:
//
//	If success, a DeleteMessageOutput object containing the result of the service call and nil.
//	Otherwise, nil and an error from the call to DeleteMessage.
func RemoveMessage(c context.Context, api SQSDeleteMessageAPI, input *sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error) {
	return api.DeleteMessage(c, input)
}

func dequeueFromSqs(ctx *context.Context, queueUrl string) (OllamaRequestMessage, error) {
	cfg, err := config.LoadDefaultConfig(*ctx)
	client := sqs.NewFromConfig(cfg)

	gMInput := &sqs.ReceiveMessageInput{
		MessageAttributeNames: []string{
			string(types.QueueAttributeNameAll),
		},
		QueueUrl:            &queueUrl,
		MaxNumberOfMessages: 1,
		VisibilityTimeout:   60,
	}

	msgResult, err := GetMessages(*ctx, client, gMInput)
	if err != nil {
		return OllamaRequestMessage{}, err
	}

	if msgResult.Messages != nil && len(msgResult.Messages) > 0 {
		fmt.Println("Message ID:   " + *msgResult.Messages[0].MessageId)
		fmt.Println("Message Body: " + *msgResult.Messages[0].Body)

		rawMessage := &msgResult.Messages[0]

		var ollamaRequestMessage OllamaRequestMessage
		err := json.Unmarshal([]byte(*rawMessage.Body), &ollamaRequestMessage)
		if err != nil {
			log.Fatalln("Error parsing message")
			return OllamaRequestMessage{}, err
		}
		cfg, _ := config.LoadDefaultConfig(*ctx)
		client := sqs.NewFromConfig(cfg)
		_queueUrl := queueUrl
		RemoveMessage(*ctx, client, &sqs.DeleteMessageInput{
			QueueUrl:      &_queueUrl,
			ReceiptHandle: rawMessage.ReceiptHandle,
		})
		return ollamaRequestMessage, nil
	} else {
		fmt.Print(".")
		return OllamaRequestMessage{}, err
	}
}

func dequeueFromRedis(ctx *context.Context, queueUrl string) (OllamaRequestMessage, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis-n3r.n3r-project-test-sb.svc.mdpb1.io.navercorp.com:6379",
		Password: "default", // no password set
		DB:       0,         // use default DB
	})

	rawMessage, err := rdb.LPop(*ctx, "review-request").Bytes()
	if err != nil {
		return OllamaRequestMessage{}, err
	}
	fmt.Printf("rawMessage = %s\n", string(rawMessage))

	var ollamaRequestMessage OllamaRequestMessage
	err = json.Unmarshal(rawMessage, &ollamaRequestMessage)
	if err != nil {
		log.Fatalln("Error parsing message")
		return OllamaRequestMessage{}, err
	}
	return ollamaRequestMessage, nil
}

type OllamaRequestMessage struct {
	RunNumber int
	PrNumber  int
	OwnerName string // repository owner
	RepoName  string // repository name
	BaseRef   string // base branch
	HeadRef   string // head branch
	PromptUrl string // S3 URL that contains the prompt
}

// TODO: Read this value from an environment variable
const queueUrl string = "https://sqs.ap-northeast-2.amazonaws.com/236145864830/auto-code-review"

func processMessage(ctx *context.Context, message OllamaRequestMessage) {

	promptString, err := downloadPrompt(ctx, message.PromptUrl)
	if err != nil {
		panic(err)
	}
	log.Printf("Prompt:\n%s\n\n", promptString)

	out, err := exec.Command("ollama", "run", "magicoder", string(promptString)).Output()
	if err != nil {
		panic(err)
	}
	reviewResult := string(out)

	log.Printf("Review:\n%s\n", reviewResult)

	compareLink := fmt.Sprintf("[%s...%s](/%s/%s/compare/%s...%s)",
		message.BaseRef, message.HeadRef,
		message.OwnerName, message.RepoName,
		message.BaseRef, message.HeadRef)
	reviewComment := fmt.Sprintf("This is an auto-generated code review for %s\n\n%s",
		compareLink, reviewResult)
	writeComment(
		ctx,
		message.OwnerName,
		message.RepoName,
		message.PrNumber,
		reviewComment)
}

func authGitHubApp(ctx *context.Context) *github.Client {
	// TODO: Read this value from an environment variable
	appId := int64(788325)
	// TODO: Read this value from an environment variable
	installationId := int64(45833995)
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appId, installationId, "ollama-reviewer.2024-01-06.private-key.pem")

	if err != nil {
		panic(err)
	}

	// Use installation transport with client.
	client := github.NewClient(&http.Client{Transport: itr})

	return client
}

func writeComment(ctx *context.Context, owner string, repo string, prNumber int, comment string) {
	client := authGitHubApp(ctx)

	input := github.IssueComment{Body: github.String(comment)}
	createdComment, _, err := client.Issues.CreateComment(*ctx, "suminb", repo, prNumber, &input)
	if err != nil {
		log.Fatalf("Issues.CreateComment returned error: %v", err)
	}
	log.Printf("%v\n", github.Stringify(createdComment))
}

func main() {
	ctx := context.Background()

	// for {
	// 	val, err := rdb.LPop(ctx, "foo").Result()
	// 	if err != nil {
	// 		fmt.Print(".")
	// 		time.Sleep(1 * time.Second)
	// 	} else {
	// 		fmt.Printf("%s\n", val)
	// 	}
	// }

	for {
		message, err := dequeueFromRedis(&ctx, "redis-n3r.n3r-project-test-sb.svc.mdpb1.io.navercorp.com:6379")
		if err != nil {
			fmt.Print(".")
			time.Sleep(1 * time.Second)
		} else {
			fmt.Printf("message = %v", message)
			processMessage(&ctx, message)
		}
	}

	// 	r := gin.Default()
	// 	r.GET("/ping", func(c *gin.Context) {
	// 		c.JSON(http.StatusOK, gin.H{
	// 			"message": "pong",
	// 		})
	// 	})
	// 	r.POST("/query", func(c *gin.Context) {
	// 		err := c.Request.ParseForm()
	// 		if err != nil {
	// 			handleError(c, err)
	// 			return
	// 		}
	// 		promptUrl := c.Request.Form.Get("promptUrl")

	// 		promptString, err := downloadPrompt(ctx, promptUrl)
	// 		if err != nil {
	// 			handleError(c, err)
	// 			return
	// 		}

	// 		out, err := exec.Command("ollama", "run", "dolphin-mixtral", string(promptString)).Output()
	// 		if err != nil {
	// 			handleError(c, err)
	// 			return
	// 		}

	//		c.JSON(http.StatusOK, gin.H{
	//			"message": string(out),
	//		})
	//	})
	//
	// r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
