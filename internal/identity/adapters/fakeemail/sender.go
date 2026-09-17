package fakeemail

import (
	"context"
	"sync"

	"github.com/AlexandreZanata/Goyim-Arena/internal/identity/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/identity/domain"
)

// SentVerificationEmail records an email message sent to a user.
type SentVerificationEmail struct {
	Email domain.Email
	Token string
}

// Sender implements application.EmailSender in-memory for testing and non-production environments.
type Sender struct {
	mu     sync.Mutex
	emails []SentVerificationEmail
	failOn error
}

var _ application.EmailSender = (*Sender)(nil)

// NewSender constructs a new fake email sender.
func NewSender() *Sender {
	return &Sender{}
}

// SendVerificationEmail records the sent verification email.
func (s *Sender) SendVerificationEmail(ctx context.Context, email domain.Email, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failOn != nil {
		return s.failOn
	}

	s.emails = append(s.emails, SentVerificationEmail{
		Email: email,
		Token: token,
	})
	return nil
}

// SentEmails returns a copy of all recorded emails.
func (s *Sender) SentEmails() []SentVerificationEmail {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]SentVerificationEmail, len(s.emails))
	copy(out, s.emails)
	return out
}

// LastTokenForEmail finds the most recent verification token sent to the given email address.
func (s *Sender) LastTokenForEmail(email domain.Email) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.emails) - 1; i >= 0; i-- {
		if s.emails[i].Email.Equals(email) {
			return s.emails[i].Token, true
		}
	}
	return "", false
}

// SetFailure configures the sender to return an error on subsequent send attempts.
func (s *Sender) SetFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failOn = err
}

// Reset clears all recorded messages and error states.
func (s *Sender) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emails = nil
	s.failOn = nil
}
