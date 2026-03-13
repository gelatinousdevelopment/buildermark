//go:build linux

package handler

import (
	"log"
	"os/exec"
	"sync"
)

var (
	notifySendPath string
	notifySendOnce sync.Once
)

func showDesktopNotification(title, body string) {
	notifySendOnce.Do(func() {
		path, err := exec.LookPath("notify-send")
		if err != nil {
			log.Printf("notify-send not found; desktop notifications disabled")
			return
		}
		notifySendPath = path
	})
	if notifySendPath == "" {
		return
	}
	cmd := exec.Command(notifySendPath, "--app-name=Buildermark", title, body)
	if err := cmd.Start(); err != nil {
		return
	}
	go cmd.Wait()
}
