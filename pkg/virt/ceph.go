package virt

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

var cephKeyringKeyPattern = regexp.MustCompile(`(?i)^\s*key\s*=\s*(\S+)\s*$`)

func cephSecretValueFromFileContent(content []byte) string {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		matches := cephKeyringKeyPattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		return strings.TrimSpace(matches[1])
	}

	if len(lines) > 1 {
		return ""
	}

	return trimmed
}

func cephSecretValueBytesFromFileContent(content []byte) ([]byte, error) {
	secretValue := cephSecretValueFromFileContent(content)
	if secretValue == "" {
		return nil, fmt.Errorf("ceph key file is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(secretValue)
	if err != nil {
		return nil, fmt.Errorf("invalid ceph key format: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("ceph key file is empty")
	}
	return decoded, nil
}

func (lve *LibVirtEp) EnsureCephSecret(secret libvirtxml.Secret, keyFile string) (err error) {
	if lve == nil || lve.Com == nil {
		return fmt.Errorf("libvirt connection is nil")
	}
	if strings.TrimSpace(secret.UUID) == "" {
		return fmt.Errorf("ceph secret uuid is required")
	}
	if strings.TrimSpace(keyFile) == "" {
		return fmt.Errorf("ceph key file is required")
	}

	secretXML, err := secret.Marshal()
	if err != nil {
		return err
	}

	sec, err := lve.Com.LookupSecretByUUIDString(secret.UUID)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return err
		}
		secretRef, defineErr := lve.Com.SecretDefineXML(secretXML, libvirt.SECRET_DEFINE_VALIDATE)
		if defineErr != nil {
			return defineErr
		}
		sec = secretRef
	}
	defer func() {
		if freeErr := sec.Free(); freeErr != nil {
			err = errors.Join(err, freeErr)
		}
	}()

	value, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	secretValue, err := cephSecretValueBytesFromFileContent(value)
	if err != nil {
		return err
	}
	if err := sec.SetValue(secretValue, 0); err != nil {
		return err
	}
	return nil
}

func (lve *LibVirtEp) RemoveCephSecretByUUID(uuid string) (err error) {
	if lve == nil || lve.Com == nil {
		return nil
	}
	if strings.TrimSpace(uuid) == "" {
		return nil
	}

	sec, err := lve.Com.LookupSecretByUUIDString(uuid)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil
		}
		return err
	}
	defer func() {
		if freeErr := sec.Free(); freeErr != nil {
			err = errors.Join(err, freeErr)
		}
	}()
	if err := sec.Undefine(); err != nil {
		return err
	}
	return nil
}
