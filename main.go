package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/matu6968/s3-client/s3client"
)

func main() {
	filePath := flag.String("file", "", "Path to file to upload")
	forcePathStyle := flag.Bool("force-path-style", false, "Enable S3 force path style")
	configPath := flag.String("config", "", "Path to config file")
	directory := flag.String("directory", "", "Directory in bucket")
	listFiles := flag.Bool("list", false, "List files in bucket")
	deleteFile := flag.String("delete", "", "Delete file from bucket")
	overwrite := flag.Bool("overwrite", false, "Overwrite existing file")
	flag.Parse()

	ctx := context.TODO()

	client, err := s3client.LoadClient(ctx, *configPath, *forcePathStyle)
	if err != nil {
		fmt.Println("Error initializing client:", err)
		os.Exit(1)
	}

	if *listFiles {
		if err := client.ListFiles(ctx); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		return
	}

	if *deleteFile != "" {
		if err := client.DeleteFile(ctx, *deleteFile); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		return
	}

	if *filePath != "" {
		url, err := client.UploadFile(ctx, *filePath, *directory, *overwrite)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		fmt.Println("Uploaded:", url)
		return
	}

	fmt.Println("No action specified. Use -file, -list, or -delete.")
}

