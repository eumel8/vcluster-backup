// vcluster-backup.go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"path/filepath"

	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func encryptFile(filename string, data []byte, passphrase string) error {
	block, err := aes.NewCipher([]byte(passphrase))
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

func main() {

	var backupFile, bucketName, accessKey, secretKey, endpoint, region, encKey string
	var backupInterval int
	var decrypt bool

	// Command-line flags for the backup file, interval, and S3 bucket name
	// File to backup, e.g. sqlite database
	flag.StringVar(&backupFile, "backupFile", "/data/server/db/state.db", "Sqlite database of K3S instance.")
	// Set the interval for backup in minutes
	flag.IntVar(&backupInterval, "backupInterval", 2, "Interval in minutes for backup.")
	// Set the S3 bucket name and key for storing the backup
	flag.StringVar(&bucketName, "bucketName", "k3s-backup", "S3 bucket name.")
	flag.StringVar(&accessKey, "accessKey", "", "S3 accesskey.")
	flag.StringVar(&secretKey, "secretKey", "", "S3 secretkey.")
	flag.StringVar(&endpoint, "endpoint", "", "S3 endpoint.")
	flag.StringVar(&region, "region", "default", "S3 region.")
	flag.StringVar(&encKey, "encKey", "", "S3 encryption key.")
	/// Calling decrypt function
	flag.BoolVar(&decrypt, "decrypt", false, "Decrypt the file")
	// Parse the command-line flags
	flag.Parse()

	if decrypt {
		fmt.Println("Decrypting file ", backupFile)

		ciphertext, err := os.ReadFile(backupFile)
		if err != nil {
			log.Println("Failed to read file for decrypt:", err)
			os.Exit(1)
		}

		plaintext, err := decryptFileAES256(backupFile, ciphertext, encKey)
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

	// Create a new minio service client
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: region,
		Secure: true,
	})

	if err != nil {
		log.Fatalln(err)
	}

	// Enable tracing.
	minioClient.TraceOn(os.Stdout)

	// Create a channel to receive termination signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	// Start a goroutine to perform the backup
	go func() {
		for {
			select {
			case <-time.After(time.Duration(backupInterval) * time.Minute):
				// Open the file to be backed up
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
				log.Println("Backup successfully created and uploaded to S3")

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