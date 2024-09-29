package main

import (
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

	if *filePath == "" {
		fmt.Println("No file specified for upload. Use -file to specify a file or -list to list bucket contents.")
		fmt.Println("Use -help for more information")
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
