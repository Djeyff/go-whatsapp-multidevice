package app

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

type IAppUsecase interface {
	Login(ctx context.Context, deviceID string) (response LoginResponse, err error)
	RetryLogin(ctx context.Context, deviceID string) (response LoginResponse, err error)
	LoginWithCode(ctx context.Context, deviceID string, phoneNumber string) (loginCode string, err error)
	PasskeyChallenge(ctx context.Context, deviceID string) (response PasskeyChallengeResponse, err error)
	PasskeyResponse(ctx context.Context, deviceID string, assertion *types.WebAuthnResponse) (err error)
	PasskeyConfirm(ctx context.Context, deviceID string) (err error)
	Logout(ctx context.Context, deviceID string) (err error)
	Reconnect(ctx context.Context, deviceID string) (err error)
	Status(ctx context.Context, deviceID string) (isConnected bool, isLoggedIn bool, err error)
	FirstDevice(ctx context.Context) (response DevicesResponse, err error)
	FetchDevices(ctx context.Context) (response []DevicesResponse, err error)
}

type DevicesResponse struct {
	Name   string `json:"name"`
	Device string `json:"device"`
	JID    string `json:"jid"`
}

type LoginResponse struct {
	ImagePath        string    `json:"image_path"`
	Duration         int64     `json:"duration"`
	PairingSessionID string    `json:"pairing_session_id"`
	QRGeneration     int64     `json:"qr_generation"`
	EmittedAt        time.Time `json:"emitted_at"`
	ValidUntil       time.Time `json:"valid_until"`
	State            string    `json:"state"`
	ErrorCode        string    `json:"error_code,omitempty"`
}

type PasskeyChallengeResponse struct {
	Status        string                   `json:"status"` // "none" | "awaiting_response" | "awaiting_confirmation"
	Challenge     *types.WebAuthnPublicKey `json:"challenge,omitempty"`
	Code          string                   `json:"code,omitempty"`
	SkipHandoffUX bool                     `json:"skip_handoff_ux,omitempty"`
}
