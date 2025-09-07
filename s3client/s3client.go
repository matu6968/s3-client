package s3client

import (
	"bufio"
	"context"
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

type Client struct {
	S3        *s3.Client
	Bucket    string
	ReturnURL string
}

// LoadClient initializes the S3 client, preferring config file but falling back to default AWS chain.
func LoadClient(ctx context.Context, configPath string, forcePathStyle bool) (*Client, error) {
	// Default config search
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			candidates := []string{
				"s3config.toml",
				filepath.Join(homeDir, ".config", "s3-client", "s3config.toml"),
			}
			for _, loc := range candidates {
				if _, err := os.Stat(loc); err == nil {
					configPath = loc
					break
				}
			}
		}
	}

	var (
		accessKey, secretKey, region, bucket, endpoint, returnURL string
	)

	if configPath != "" {
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err == nil {
			accessKey = viper.GetString("aws_access_key_id")
			secretKey = viper.GetString("aws_secret_access_key")
			region = viper.GetString("region")
			bucket = viper.GetString("bucket")
			endpoint = viper.GetString("endpoint")
			returnURL = viper.GetString("returnurl")
		}
	}

	// Custom endpoint resolver if provided
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, opts ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" && service == s3.ServiceID {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	// Build config
	var cfg aws.Config
	var err error
	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
			config.WithEndpointResolverWithOptions(customResolver),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithEndpointResolverWithOptions(customResolver),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = forcePathStyle
	})

	return &Client{
		S3:        s3client,
		Bucket:    bucket,
		ReturnURL: returnURL,
	}, nil
}

// UploadFile uploads a file with overwrite confirmation
func (c *Client) UploadFile(ctx context.Context, filePath, directory string, overwrite bool) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	key := fileInfo.Name()
	if directory != "" {
		dir := strings.Trim(directory, "/")
		key = filepath.Join(dir, key)
	}
	key = filepath.ToSlash(key)

	// Check existence
	_, err = c.S3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	})
	if err == nil && !overwrite {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("File already exists. Overwrite? [y/n] > ")
		resp, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(resp)) != "y" {
			return "", fmt.Errorf("upload cancelled by user")
		}
	}

	uploader := manager.NewUploader(c.S3)
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		return "", fmt.Errorf("uploading file: %w", err)
	}

	fullURL := fmt.Sprintf("%s/%s", strings.TrimRight(c.ReturnURL, "/"), strings.TrimLeft(key, "/"))
	return fullURL, nil
}

// ListFiles lists all objects in the bucket
func (c *Client) ListFiles(ctx context.Context) error {
	paginator := s3.NewListObjectsV2Paginator(c.S3, &s3.ListObjectsV2Input{Bucket: &c.Bucket})
	fmt.Printf("Files in bucket '%s':\n", c.Bucket)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("listing files: %w", err)
		}
		for _, item := range page.Contents {
			fmt.Printf("- %s (Size: %d, Last modified: %s)\n",
				aws.ToString(item.Key), item.Size, item.LastModified.Format("2006-01-02 15:04:05"))
		}
	}
	return nil
}

// DeleteFile deletes a file and waits until it is gone
func (c *Client) DeleteFile(ctx context.Context, key string) error {
	key = strings.TrimPrefix(key, "/")
	_, err := c.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}

	waiter := s3.NewObjectNotExistsWaiter(c.S3)
	if err := waiter.Wait(ctx, &s3.HeadObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	}, 0); err != nil {
		return fmt.Errorf("waiting for deletion: %w", err)
	}

	fmt.Printf("Deleted: %s\n", key)
	return nil
}
