package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
	watcher  *fsnotify.Watcher
	filePath string
	fileName string
}

func New(filePath string) (*FileWatcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dirPath := filepath.Dir(filePath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("Dir %s not exists, creating...", dirPath)
		if err := os.MkdirAll(dirPath, 0777); err != nil {
			fsWatcher.Close()
			return nil, err
		}
	}

	if err := fsWatcher.Add(dirPath); err != nil {
		fsWatcher.Close()
		return nil, err
	}

	log.Printf("Start listening dir: %s", dirPath)

	return &FileWatcher{
		watcher:  fsWatcher,
		filePath: filePath,
		fileName: filepath.Base(filePath),
	}, nil
}

func (fw *FileWatcher) Start(ctx context.Context) <-chan []byte {
	contentChan := make(chan []byte)

	go func() {
		defer func() {
			fw.watcher.Close()
			close(contentChan)
			log.Printf("File watcher closed")
		}()

		for {
			select {
			case event, ok := <-fw.watcher.Events:
				if !ok {
					log.Printf("watcher events not ok")
					return
				}

				eventFileName := filepath.Base(event.Name)

				if eventFileName == fw.fileName &&
					(event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write) {

					content, err := os.ReadFile(fw.filePath)
					if err != nil {
						log.Println("Read file err:", err)
					} else {
						log.Printf("Get new content: %s", string(content))
						contentChan <- content
					}
				}

			case err, ok := <-fw.watcher.Errors:
				if !ok {
					log.Printf("watcher events not ok")
					return
				}
				log.Println("watcher error:", err)

			case <-ctx.Done():
				log.Println("Receive close signal, closing watcher")
				return
			}
		}
	}()

	return contentChan
}
