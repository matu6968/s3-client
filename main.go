package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	verbose := flag.Bool("v", false, "Enable verbose output")
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
			fmt.Println("No config file found. Please specify a config file using -config or create one in the default locations.")
			os.Exit(1)
		}
	}

	viper.SetConfigFile(*configPath)
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file: %s\n", err)
		os.Exit(1)
	}

	accessKey := viper.GetString("aws_access_key_id")
	secretKey := viper.GetString("aws_secret_access_key")
	region := viper.GetString("region")
	bucket := viper.GetString("bucket")
	endpoint := viper.GetString("endpoint")
	returnurl := viper.GetString("returnurl")

	// Build AWS config
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, opts ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" && service == s3.ServiceID {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		fmt.Printf("Error loading AWS config: %s\n", err)
		os.Exit(1)
	}

	svc := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = *forcePathStyle
	})

	if *listFiles {
		listBucketFiles(context.TODO(), svc, bucket)
		return
	}

	if *deleteFile != "" {
		deleteS3File(context.TODO(), svc, bucket, *deleteFile)
		return
	}

	if *filePath == "" {
		fmt.Println("No file specified for upload. Use -file to specify a file or -list to list bucket contents.")
		os.Exit(1)
	}
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

	// HeadObject check
	_, err = svc.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err == nil && !*overwrite {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("The file already exists. Overwrite? [y/n] > ")
		response, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("Upload cancelled.")
			os.Exit(0)
		}
	}

	uploader := manager.NewUploader(svc)
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		fmt.Printf("Error uploading file to S3: %s\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Uploaded file: %s\n", *filePath)
		fmt.Printf("Endpoint: %s\n", endpoint)
	}

	fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(returnurl, "/"), strings.TrimLeft(key, "/"))
	fmt.Printf("Successfully uploaded. File URL: %s\n", fullURL)
}

func listBucketFiles(ctx context.Context, svc *s3.Client, bucket string) {
	paginator := s3.NewListObjectsV2Paginator(svc, &s3.ListObjectsV2Input{Bucket: &bucket})
	fmt.Printf("Files in bucket '%s':\n", bucket)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			fmt.Printf("Error listing files: %s\n", err)
			os.Exit(1)
		}
		for _, item := range page.Contents {
			fmt.Printf("- %s (Size: %d bytes, Last modified: %s)\n",
				aws.ToString(item.Key),
				item.Size,
				item.LastModified.Format("2006-01-02 15:04:05"))
		}
	}
}

func deleteS3File(ctx context.Context, svc *s3.Client, bucket, filePath string) {
	filePath = strings.TrimPrefix(filePath, "/")
	_, err := svc.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &filePath,
	})
	if err != nil {
		fmt.Printf("Error deleting file: %s\n", err)
		os.Exit(1)
	}

	// Waiter
	waiter := s3.NewObjectNotExistsWaiter(svc)
	err = waiter.Wait(ctx, &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &filePath,
	}, 0)
	if err != nil {
		fmt.Printf("Error waiting for file deletion: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully deleted file: %s\n", filePath)
}

