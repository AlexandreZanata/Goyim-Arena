package outbox

import (
	"context"
	"fmt"

	identityapp "github.com/AlexandreZanata/Goyim-Arena/internal/identity/application"
	identitydomain "github.com/AlexandreZanata/Goyim-Arena/internal/identity/domain"
	"github.com/AlexandreZanata/Goyim-Arena/internal/notifications/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/notifications/domain"
)

// Sender satisfies the identity module's email port by queueing the message
// instead of sending it (P15-T04).
//
// This is the bridge pattern the codebase already uses where one module needs
// another's capability: identity declares what it wants ("deliver or enqueue
// this verification email"), and this adapter decides how, without identity
// learning anything about queues, templates or a provider.
//
// The port's methods return an error and the flows propagate it, so a queue
// that cannot accept the job fails the operation that was creating the event.
// That is the deliberate choice: an account created in a state where its
// verification message was silently dropped would be a user with no way in.
type Sender struct {
	notifier *application.Notifier
}

var _ identityapp.EmailSender = (*Sender)(nil)

// NewSender wires the bridge. A nil notifier fails at construction.
func NewSender(notifier *application.Notifier) (*Sender, error) {
	if notifier == nil {
		return nil, domain.ErrMissingDependency
	}
	return &Sender{notifier: notifier}, nil
}

// SendVerificationEmail queues the confirmation of an email address.
func (s *Sender) SendVerificationEmail(ctx context.Context, email identitydomain.Email, token string) error {
	return s.queue(ctx, domain.TemplateVerification, email, token)
}

// SendPasswordResetEmail queues the password recovery code.
func (s *Sender) SendPasswordResetEmail(ctx context.Context, email identitydomain.Email, token string) error {
	return s.queue(ctx, domain.TemplatePasswordReset, email, token)
}

func (s *Sender) queue(ctx context.Context, templateID domain.TemplateID, email identitydomain.Email, token string) error {
	if s == nil || s.notifier == nil {
		return domain.ErrMissingDependency
	}
	address := email.String()
	if address == "" {
		return domain.ErrInvalidRecipient
	}
	// The code is the runtime value of the template; the display name is not
	// known at this layer, so the greeting degrades to its generic form
	// instead of inventing one.
	values, err := domain.NewTemplateValues("", token)
	if err != nil {
		return err
	}
	if _, err := s.notifier.Notify(ctx, application.NotificationRequest{
		Template:  templateID,
		Recipient: address,
		Values:    values,
	}); err != nil {
		// The template name is safe to name; the address and the code are not.
		return fmt.Errorf("outbox: queue %s: %w", templateID, err)
	}
	return nil
}
