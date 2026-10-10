package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jaisheesh-2006/ChitSetu/middleware"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type User struct {
	UserID      string    `bson:"_id" json:"user_id"`
	Email       string    `bson:"email" json:"email"`
	FullName    string    `bson:"full_name,omitempty" json:"full_name,omitempty"`
	AvatarURL   string    `bson:"avatar_url,omitempty" json:"avatar_url,omitempty"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	LastLoginAt time.Time `bson:"last_login_at" json:"last_login_at"`
}
 
type UserStore struct {
	usersCol *mongo.Collection
}

func NewUserStore(db *mongo.Database) *UserStore {
	return &UserStore{usersCol: db.Collection("users")}
}

// UpsertFromSupabase creates the user on first login and updates profile fields and last_login_at on every login.
func (s *UserStore) UpsertFromSupabase(ctx context.Context, claims *middleware.SupabaseClaims) (*User, error) {
	if claims == nil {
		return nil, fmt.Errorf("claims are required")
	}

	userID := strings.TrimSpace(claims.Subject)
	if userID == "" {
		return nil, fmt.Errorf("claims subject is required")
	}

	now := time.Now().UTC()
	email := strings.TrimSpace(strings.ToLower(claims.Email))
	fullName := strings.TrimSpace(readMetadataString(claims.UserMetadata, "full_name", "name"))
	avatarURL := strings.TrimSpace(readMetadataString(claims.UserMetadata, "avatar_url", "picture"))

	if user, err := s.findOne(ctx, bson.M{"_id": userID}); err != nil {
		return nil, err
	} else if user != nil {
		if err := s.updateSupabaseUser(ctx, user.UserID, email, fullName, avatarURL, now, userID); err != nil {
			return nil, err
		}
		return s.GetByID(ctx, user.UserID)
	}

	if email != "" {
		if user, err := s.findOne(ctx, bson.M{"email": email}); err != nil {
			return nil, err
		} else if user != nil {
			if err := s.updateSupabaseUser(ctx, user.UserID, email, fullName, avatarURL, now, userID); err != nil {
				return nil, err
			}
			return s.GetByID(ctx, user.UserID)
		}
	}

	if err := s.insertSupabaseUser(ctx, userID, email, fullName, avatarURL, now); err != nil {
		return nil, err
	}

	return s.GetByID(ctx, userID)
}

func (s *UserStore) findOne(ctx context.Context, filter bson.M) (*User, error) {
	var user User
	err := s.usersCol.FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{
		"_id":          1,
		"email":         1,
		"full_name":     1,
		"avatar_url":    1,
		"created_at":    1,
		"last_login_at": 1,
	})).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &user, nil
}

func (s *UserStore) updateSupabaseUser(ctx context.Context, userID, email, fullName, avatarURL string, now time.Time, googleSub string) error {
	_, err := s.usersCol.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{
			"email":         email,
			"full_name":     fullName,
			"avatar_url":    avatarURL,
			"last_login_at": now,
			"updated_at":    now,
			"google_sub":    googleSub,
			"auth_provider": "google",
		}},
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

func (s *UserStore) insertSupabaseUser(ctx context.Context, userID, email, fullName, avatarURL string, now time.Time) error {
	_, err := s.usersCol.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"email":         email,
				"full_name":     fullName,
				"avatar_url":    avatarURL,
				"last_login_at": now,
				"updated_at":    now,
			},
			"$setOnInsert": bson.M{
				"created_at":        now,
				"auth_provider":     "google",
				"profile_completed": false,
				"password_hash":     "",
				"google_sub":        userID,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

func (s *UserStore) GetByID(ctx context.Context, userID string) (*User, error) {
	user, err := s.findOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func readMetadataString(metadata map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if metadata == nil {
			continue
		}
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}
