// vcluster-backup.go
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"path/filepath"

	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Encrypts the given data using AES-256-GCM and writes it to the file
func encryptFileAES256(filename string, data []byte, passphrase string) error {
	// Generate a 32-byte key from the passphrase
	hasher := sha256.New()
	hasher.Write([]byte(passphrase))
	key := hasher.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return os.WriteFile(filename, ciphertext, 0777)
}

// Decrypts the given file using AES-256-GCM and returns the decrypted data
func decryptFileAES256(filename string, ciphertext []byte, passphrase string) ([]byte, error) {
	// Generate a 32-byte key from the passphrase
	hasher := sha256.New()
	hasher.Write([]byte(passphrase))
	key := hasher.Sum(nil)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, err
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func listS3Objects(ctx context.Context, s3Client *minio.Client, bucketName string) ([]minio.ObjectInfo, error) {
	var objects []minio.ObjectInfo
	doneCh := make(chan struct{})
	defer close(doneCh)

	for object := range s3Client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{}) {
		if object.Err != nil {
			return nil, object.Err
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func minioClient(endpoint, accessKey, secretKey, region string, trace string, insecure string) (*minio.Client, error) {

	s := true
	if insecure != "" {
		s = false
	}
	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: region,
		Secure: s,
	})
	if err != nil {
		return nil, err
	}

	// Enable tracing of S3 API calls
	if trace != "" {
		minioClient.TraceOn(os.Stdout)
	}

	return minioClient, nil
}

func main() {
	// Command-line flags for the backup file, interval, and S3 bucket name
	var backupFile, bucketName, accessKey, secretKey, endpoint, region, encKey, backupInterval string
	var restore, list bool

	trace := ""
	insecure := ""

	// parse command-line flags
	flag.StringVar(&backupFile, "backupFile", "/data/server/db/state.db", "Sqlite database of K3S instance. (ENV BACKUP_FILE)")
	flag.StringVar(&backupInterval, "backupInterval", os.Getenv("BACKUP_INTERVAL"), "Interval in minutes for backup. (ENV BACKUP_INTERVAL)")
	flag.StringVar(&bucketName, "bucketName", os.Getenv("BUCKET_NAME"), "S3 bucket name. (ENV BUCKET_NAME)")
	flag.StringVar(&accessKey, "accessKey", os.Getenv("ACCESS_KEY"), "S3 accesskey. (ENV ACCESS_KE)")
	flag.StringVar(&secretKey, "secretKey", os.Getenv("SECRET_KEY"), "S3 secretkey. (ENV SECRET_KEY)")
	flag.StringVar(&endpoint, "endpoint", os.Getenv("ENDPOINT"), "S3 endpoint. (ENV ENDPOINT)")
	flag.StringVar(&region, "region", os.Getenv("REGION"), "S3 region. (ENV REGION)")
	flag.StringVar(&encKey, "encKey", os.Getenv("ENC_KEY"), "S3 encryption key. (ENV ENC_KEY)")
	// Trace S3 API calls
	flag.StringVar(&trace, "trace", os.Getenv("TRACE"), "Trace S3 API calls (trace=1). (ENV TRACE)")
	// insecure S3 connection
	flag.StringVar(&insecure, "insecure", os.Getenv("INSECURE"), "Insecure S3 API calls (insecure=1). (ENV INSECURE)")
	// Calling decrypt function for backup restore
	flag.BoolVar(&restore, "restore", false, "Restore and decrypt S3 backup file")
	// Calling S3object list function
	flag.BoolVar(&list, "list", false, "List S3 objects")
	flag.Parse()

	// set default backup interval if not set to 60 (min)
	if backupInterval == "" {
		backupInterval = "60"
	}

	// set default backup file if not set (K3S sqlite db)
	if backupFile == "" {
		backupFile = "/data/server/db/state.db"
	}

	// print program start information
	log.Println("Welcome to vCluster backup")
	log.Println("S3 endpoint:", endpoint)
	log.Println("S3 bucketName:", bucketName)
	log.Println("BackupFile:", backupFile)
	log.Println("S3 accessKey:", accessKey)
	log.Println("S3 secretKey:", secretKey[0:2], "...")
	log.Println("S3 region:", region)
	log.Println("encKey:", encKey[0:2], "...")
	log.Println("S3 trace: ", trace)
	log.Println("S3 insecure: ", insecure)
	log.Println("backupInterval: ", backupInterval)

	// Init Minio client for S3 backend operations
	minioClient, err := minioClient(endpoint, accessKey, secretKey, region, trace, insecure)
	if err != nil {
		log.Println("Failed to create MinIO client:", err)
		os.Exit(1)
	}

	// list backups
	if list {
		fmt.Println("Listing S3 objects in bucket ", bucketName)
		objects, err := listS3Objects(context.Background(), minioClient, bucketName)
		if err != nil {
			log.Println("Failed to list S3 objects:", err)
			os.Exit(1)
		}

		for _, object := range objects {
			fmt.Printf("Object: %s\n", object.Key)
		}
		os.Exit(0)
	}

	// restore selected backup
	if restore {
		fmt.Println("Fetch & Decrypting file ", backupFile)

		// Fetch the object from S3
		fetchedObject, err := minioClient.GetObject(context.Background(), bucketName, backupFile, minio.GetObjectOptions{})
		if err != nil {
			log.Println("Failed to fetch object from S3:", err)
			os.Exit(1)
		}

		var ciphertext bytes.Buffer
		_, err = io.Copy(&ciphertext, fetchedObject)
		if err != nil {
			log.Println("Failed to read file for decrypt:", err)
			os.Exit(1)
		}

		plaintext, err := func() ([]byte, error) {
			var (
				_          string = backupFile
				ciphertext []byte = ciphertext.Bytes()
				passphrase string = encKey
			)
			hasher := sha256.New()
			hasher.Write([]byte(passphrase))
			key := hasher.Sum(nil)
			block, err := aes.NewCipher(key)
			if err != nil {
				return nil, err
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return nil, err
			}
			nonceSize := gcm.NonceSize()
			if len(ciphertext) < nonceSize {
				return nil, err
			}
			nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
			plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
			if err != nil {
				return nil, err
			}
			return plaintext, nil
		}()
		if err != nil {
			log.Println("Failed to decrypt file:", err)
			os.Exit(1)
		}

		restoreFile := backupFile + ".restore"
		err = os.WriteFile(restoreFile, plaintext, 0644)
		if err != nil {
			log.Println("Failed to write decrypted file:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Create a channel to receive termination signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	// Start a goroutine to perform the backup
	log.Println("Start vcluster-backup. Perform the first backup in ", backupInterval, " minute(s)")
	backupTime, _ := strconv.Atoi(backupInterval)
	go func() {
		for {
			select {

			case <-time.After(time.Duration(backupTime) * time.Minute):
				// Open the file to be backed up
				// TODO: Might be better use sqlite3, i.e sqlite3 state.db ".backup backup/state-$(date +%Y-%m-%d-%H-%M-%S).db"
				file, err := os.Open(backupFile)
				if err != nil {
					log.Println("Failed to open file:", err)
					continue
				}
				defer file.Close()

				// Create the backup file name with timestamp
				backupFileTimestamped := fmt.Sprintf("backup_%s.db", time.Now().Format("20060102150405"))
				backupFileTimestampedEnc := backupFileTimestamped + ".enc"

				// Create the backup file in a temporary location
				log.Println("Create backup file:", backupFileTimestamped)
				backupFilePath := filepath.Join(os.TempDir(), backupFileTimestamped)
				toBackupFile, err := os.Create(backupFilePath)

				if err != nil {
					log.Println("Failed to create backup file:", err)
					continue
				}
				defer toBackupFile.Close()

				// Copy the contents of the original file to the backup file
				log.Println("Start copy content:", backupFileTimestamped)
				_, err = io.Copy(toBackupFile, file)

				if err != nil {
					log.Println("Failed to copy file contents:", err)
					continue
				}

				// Encrypt the backup file in minio
				log.Println("Encrypt file:", backupFileTimestamped)
				backupFilePathEnc := backupFilePath + ".enc"

				// Read the contents of the file into a byte slice
				fileContents, err := os.ReadFile(backupFilePath)

				//toBackupFile)
				if err != nil {
					log.Println("Failed to read file contents:", err)
					continue
				}

				err = encryptFileAES256(backupFilePathEnc, fileContents, encKey)

				if err != nil {
					log.Println("Failed to encrypt file:", err)
					continue
				}

				// Upload the backup file to S3
				_, err = minioClient.FPutObject(context.Background(), bucketName, backupFileTimestampedEnc, backupFilePathEnc, minio.PutObjectOptions{})
				if err != nil {
					log.Println("Failed to upload file:", err)
					continue
				} else {
					log.Println("Backup successfully created and uploaded to S3")
				}

				// Remove the temporary backup file
				err = os.Remove(backupFilePath)
				if err != nil {
					log.Println("Failed to remove backup file:", err)
					continue
				}
				err = os.Remove(backupFilePathEnc)
				if err != nil {
					log.Println("Failed to remove backup file:", err)
					continue
				}
				log.Println("Temporary backup file removed")

			case <-signalCh:
				// Terminate the backup process on receiving termination signal
				log.Println("Terminating backup process")
				return
			}
		}
	}()

	// Wait for termination signal
	<-signalCh
}
