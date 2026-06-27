package queue

// Routing keys — used by both publisher and worker.
const (
	EventWelcomeEmail = "email.welcome"
	EventOTPEmail     = "email.otp"
)

type WelcomeEmailPayload struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

type OTPEmailPayload struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}
