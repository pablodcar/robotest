package infra

import (
	"context"

	"github.com/gravitational/robotest/lib/wait"
	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"

	log "github.com/sirupsen/logrus"
)

// UploadUpdate uploads a new application version to the installer node
func UploadUpdate(provisioner Provisioner, installer Node) (err error) {
	var session *ssh.Session
	err = wait.Retry(context.TODO(), func() error {
		session, err = installer.Connect()
		if err != nil {
			log.Debug(trace.DebugReport(err))
		}
		return trace.Wrap(err)
	})
	if err != nil {
		errClose := session.Close()
		if errClose != nil {
			log.Errorf("failed to close upload update SSH session: %v", errClose)
		}
		return trace.Wrap(err)
	}
	defer session.Close()

	// launch the upload update script
	log.Debugf("starting uploading update process...")
	err = provisioner.UploadUpdate(session)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
