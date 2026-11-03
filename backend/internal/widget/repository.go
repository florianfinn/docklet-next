// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original encrypted widget configuration persistence.

package widget

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var silentLogger = logger.Discard

type row struct {
	Kind           dto.Kind  `gorm:"primaryKey;type:TEXT;not null"`
	Enabled        bool      `gorm:"type:INTEGER;not null;default:0"`
	Address        string    `gorm:"type:TEXT;not null;default:''"`
	CredentialEnc  *string   `gorm:"type:TEXT"`
	TLSPin         *string   `gorm:"type:TEXT"`
	OpenURL        string    `gorm:"type:TEXT;not null;default:''"`
	StackReference *string   `gorm:"type:TEXT"`
	Revision       int64     `gorm:"type:INTEGER;not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (row) TableName() string { return "service_widgets" }

// Models exposes widget persistence to the central development schema registry.
func Models() []any { return []any{&row{}} }

func load(ctx context.Context, db *database.DB, kind dto.Kind) (row, error) {
	r := row{Kind: kind}
	err := db.WithContext(ctx).Where("kind = ?", kind).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row{Kind: kind}, nil
	}
	if err != nil {
		return r, dto.Failure(dto.PersistenceFailed)
	}
	return r, nil
}

func (r row) config() dto.Config {
	c := dto.Config{Kind: r.Kind, Enabled: r.Enabled, Address: r.Address, HasCredential: r.CredentialEnc != nil, OpenURL: r.OpenURL, Revision: r.Revision, UpdatedAt: r.UpdatedAt}
	if r.TLSPin != nil {
		_ = json.Unmarshal([]byte(*r.TLSPin), &c.TLSPin)
	}
	if r.StackReference != nil {
		_ = json.Unmarshal([]byte(*r.StackReference), &c.Stack)
	}
	return c
}

func secret(r row) (string, error) {
	if r.CredentialEnc == nil {
		return "", nil
	}
	e, err := crypto.Default()
	if err != nil {
		return "", dto.Failure(dto.CredentialUnavailable)
	}
	value, err := e.Decrypt(*r.CredentialEnc)
	if err != nil || value == "" {
		return "", dto.Failure(dto.CredentialUnavailable)
	}
	return value, nil
}

func updatedRow(kind dto.Kind, old row, in dto.ConfigUpdate, now time.Time) (row, error) {
	if err := validate(kind, &in); err != nil {
		return row{}, err
	}
	r := row{Kind: kind, Enabled: in.Enabled, Address: in.Address, CredentialEnc: old.CredentialEnc, OpenURL: in.OpenURL, Revision: old.Revision + 1, UpdatedAt: now}
	if in.TLSPin != nil {
		b, _ := json.Marshal(in.TLSPin)
		s := string(b)
		r.TLSPin = &s
	}
	if in.Stack != nil {
		b, _ := json.Marshal(in.Stack)
		s := string(b)
		r.StackReference = &s
	}
	switch in.CredentialAction {
	case "clear":
		r.CredentialEnc = nil
	case "replace":
		e, err := crypto.Default()
		if err != nil {
			return row{}, dto.Failure(dto.CredentialUnavailable)
		}
		v, err := e.Encrypt(in.Credential)
		if err != nil {
			return row{}, dto.Failure(dto.CredentialUnavailable)
		}
		r.CredentialEnc = &v
	}
	return r, nil
}

func save(ctx context.Context, db *database.DB, r row) error {
	// Secrets must never be interpolated into GORM's diagnostic SQL logger.
	tx := db.WithContext(ctx).Session(&gorm.Session{Logger: silentLogger})
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}}, UpdateAll: true}).Create(&r).Error; err != nil {
		return dto.Failure(dto.PersistenceFailed)
	}
	return nil
}

func authority(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func validate(kind dto.Kind, in *dto.ConfigUpdate) error {
	bad := func() error { return dto.Failure(dto.InvalidConfig) }
	if !kind.Valid() || len(in.Address) > 2048 || len(in.OpenURL) > 2048 || len(in.Credential) > 4096 || strings.ContainsAny(in.Credential, "\r\n\x00") {
		return bad()
	}
	if in.CredentialAction == "" {
		in.CredentialAction = "keep"
	}
	if in.CredentialAction != "keep" && in.CredentialAction != "replace" && in.CredentialAction != "clear" {
		return bad()
	}
	if (in.CredentialAction == "replace") != (in.Credential != "") {
		return bad()
	}
	if kind == dto.Minecraft && (in.CredentialAction == "replace" || in.TLSPin != nil) {
		return bad()
	}
	in.Address = strings.TrimSpace(in.Address)
	if in.Enabled && in.Address == "" {
		return bad()
	}
	if in.Address != "" {
		if kind == dto.Minecraft {
			host, port, err := net.SplitHostPort(in.Address)
			if err != nil {
				host = in.Address
				port = "25565"
				if strings.ContainsAny(host, ":/ ?#@") {
					return bad()
				}
			}
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 || host == "" || len(host) > 253 || strings.ContainsAny(host, "/ ?#@\t\r\n") {
				return bad()
			}
			in.Address = net.JoinHostPort(host, port)
		} else {
			u, err := url.Parse(in.Address)
			if err != nil || !validHTTP(u) || u.RawQuery != "" || u.ForceQuery {
				return bad()
			}
			in.Address = strings.TrimRight(u.String(), "/")
			if in.TLSPin != nil {
				pin, err := hex.DecodeString(in.TLSPin.SHA256)
				if u.Scheme != "https" || err != nil || len(pin) != 32 || in.TLSPin.SHA256 != strings.ToLower(in.TLSPin.SHA256) || in.TLSPin.HostPort != authority(u) {
					return bad()
				}
			}
		}
	} else if in.TLSPin != nil {
		return bad()
	}
	if in.OpenURL != "" {
		u, err := url.Parse(in.OpenURL)
		if err != nil || !validHTTP(u) {
			return bad()
		}
	}
	if in.Stack != nil && (in.Stack.EnvironmentID == "" || in.Stack.ProjectID == "" || len(in.Stack.EnvironmentID) > 200 || len(in.Stack.ProjectID) > 200) {
		return bad()
	}
	return nil
}
func validHTTP(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.Opaque == ""
}
