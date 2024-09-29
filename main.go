package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/viper"
)

func main() {
	filePath := flag.String("file", "", "Path to the file to upload")
	forcePathStyle := flag.Bool("force-path-style", false, "Enable S3 force path style")
	configPath := flag.String("config", "", "Path to the configuration file")
	directory := flag.String("directory", "", "Directory in the S3 bucket to upload the file to")
	listFiles := flag.Bool("list", false, "List files in the S3 bucket")
	deleteFile := flag.String("delete", "", "Path to the file to delete from the S3 bucket")
	overwrite := flag.Bool("overwrite", false, "Overwrite the file if it already exists on S3")
	flag.Parse()

	if *configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting user home directory: %s\n", err)
			os.Exit(1)
		}

		configLocations := []string{
			"s3config.toml",
			filepath.Join(homeDir, ".config", "s3-client", "s3config.toml"),
		}

		for _, loc := range configLocations {
			if _, err := os.Stat(loc); err == nil {
				*configPath = loc
				break
			}
		}

		if *configPath == "" {
			fmt.Println("No config file found. Please specify a config file using --config or create one in the default locations.")
			os.Exit(1)
		}
	}

	viper.SetConfigFile(*configPath)
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("Error reading config file: %s\n", err)
		fmt.Println("Please make sure the config file exists and is accessible.")
		os.Exit(1)
	}

	accessKey := viper.GetString("aws_access_key_id")
	secretKey := viper.GetString("aws_secret_access_key")
	region := viper.GetString("region")
	bucket := viper.GetString("bucket")
	endpoint := viper.GetString("endpoint")
	returnurl := viper.GetString("returnurl")

	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: forcePathStyle,
	})
	if err != nil {
		fmt.Printf("Error creating AWS session: %s\n", err)
		os.Exit(1)
	}

	svc := s3.New(sess)

	if *listFiles {
		listBucketFiles(svc, bucket)
		return
	}

	if *deleteFile != "" {
		deleteS3File(svc, bucket, *deleteFile)
		return
	}

	if *filePath == "" {
		fmt.Println("No file specified for upload. Use -file to specify a file or -list to list bucket contents.")
		fmt.Println("Use -help for more information")
		os.Exit(1)
	}

	// Check if the file exists before attempting to open it
	if _, err := os.Stat(*filePath); os.IsNotExist(err) {
		fmt.Printf("File does not exist: %s\n", *filePath)
		os.Exit(1)
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fmt.Printf("Error opening file: %s\n", err)
		os.Exit(1)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()

	key := fileInfo.Name()
	if *directory != "" {
		dir := strings.Trim(*directory, "/")
		key = filepath.Join(dir, key)
	}
	key = filepath.ToSlash(key)

	// Check if the file already exists on S3
	headObjectInput := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	_, err = svc.HeadObject(headObjectInput)
	if err == nil && !*overwrite {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("The file with the same filename already exists on S3. Do you want to overwrite? [y/n] > ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Upload cancelled.")
			os.Exit(0)
		}
	}

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
	})

	if err != nil {
		fmt.Printf("Error uploading file to S3: %s\n", err)
		os.Exit(1)
	}

	fullURL := fmt.Sprintf("%s/%s",
		strings.TrimRight(returnurl, "/"),
		strings.TrimLeft(key, "/"))

	fmt.Printf("Successfully uploaded. File URL: %s\n", fullURL)
}

func listBucketFiles(svc *s3.S3, bucket string) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	fmt.Printf("Files in bucket '%s':\n", bucket)
	err := svc.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, item := range page.Contents {
			fmt.Printf("- %s (Size: %d bytes, Last modified: %s)\n",
				*item.Key,
				*item.Size,
				item.LastModified.Format("2006-01-02 15:04:05"))
		}
		return true
	})

	if err != nil {
		fmt.Printf("Error listing files in S3 bucket: %s\n", err)
		os.Exit(1)
	}
}

func deleteS3File(svc *s3.S3, bucket, filePath string) {
	filePath = strings.TrimPrefix(filePath, "/")
	_, err := svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filePath),
	})

	if err != nil {
		fmt.Printf("Error deleting file from S3: %s\n", err)
		os.Exit(1)
	}

	err = svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filePath),
	})

	if err != nil {
		fmt.Printf("Error waiting for file deletion: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully deleted file: %s\n", filePath)
}
