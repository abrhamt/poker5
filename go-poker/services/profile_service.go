package services

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

// AvatarCount is how many predefined avatars the client ships (PRESETS in
// frontend/src/components/PlayerAvatar.tsx). Keep the two in step.
const AvatarCount = 12

var (
	ErrInvalidAvatar   = errors.New("unknown avatar")
	ErrInvalidUsername = errors.New("usernames are 3–20 letters, numbers or underscores")
	ErrSeatedRename    = errors.New("leave your table before changing your username")
	ErrWrongPassword   = errors.New("your current password is incorrect")
	ErrSamePassword    = errors.New("choose a password different from your current one")
	ErrUsernameTaken   = errors.New("that username is taken")
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

// InputError is a rejection whose message is written for the player (a
// password rule, say), as opposed to a fault the client should not see.
type InputError struct{ msg string }

func (e *InputError) Error() string { return e.msg }

// ProfileService is how a signed-in player changes their own account.
type ProfileService struct {
	q    *repository.Queries
	game *GameService
}

func NewProfileService(q *repository.Queries, game *GameService) *ProfileService {
	return &ProfileService{q: q, game: game}
}

// SetAvatar picks one of the predefined avatars, or -1 to go back to the one
// derived from the username. It shows at their table straight away.
func (s *ProfileService) SetAvatar(ctx context.Context, user *repository.User, avatar int) error {
	if avatar < -1 || avatar >= AvatarCount {
		return ErrInvalidAvatar
	}
	if err := s.q.UpdateUserAvatar(ctx, repository.UpdateUserAvatarParams{Avatar: int16(avatar), ID: user.ID}); err != nil {
		return err
	}
	if s.game != nil {
		s.game.SetPlayerAvatar(user.ID, int16(avatar))
	}
	return nil
}

// SetUsername renames the player. Not while they hold a seat: the table knows
// its players by name, so a rename mid-session would orphan their seat.
func (s *ProfileService) SetUsername(ctx context.Context, user *repository.User, username string) error {
	username = strings.TrimSpace(username)
	if username == user.Username {
		return nil
	}
	if !usernamePattern.MatchString(username) {
		return ErrInvalidUsername
	}
	if s.game != nil && s.game.IsSeated(user.ID) {
		return ErrSeatedRename
	}
	if existing, err := s.q.GetUserByUsername(ctx, username); err == nil && existing.ID != user.ID {
		return ErrUsernameTaken
	}
	// A live signup holds its username just as an account does.
	if pending, err := s.q.GetPendingRegistrationByUsername(ctx, username); err == nil && pending.ExpiresAt > time.Now().Unix() {
		return ErrUsernameTaken
	}
	if err := s.q.UpdateUsername(ctx, repository.UpdateUsernameParams{Username: username, ID: user.ID}); err != nil {
		// The unique index is the final word if two renames race.
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrUsernameTaken
		}
		return err
	}
	return nil
}

// SetPassword changes the password after checking the current one, then signs
// out every other device; the session making the change stays signed in.
func (s *ProfileService) SetPassword(ctx context.Context, user *repository.User, sessionToken, current, next string) error {
	if !utilities.CheckPassword(current, user.PasswordHash) {
		return ErrWrongPassword
	}
	if current == next {
		return ErrSamePassword
	}
	if err := utilities.ValidatePassword(next); err != nil {
		return &InputError{msg: err.Error()}
	}
	hash, err := utilities.HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{PasswordHash: hash, ID: user.ID}); err != nil {
		return err
	}
	return s.q.DeleteOtherSessionTokens(ctx, repository.DeleteOtherSessionTokensParams{UserID: user.ID, SessionToken: sessionToken})
}
