package matchmaking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"langapp-backend/websocket"
)

type MatchingService struct {
	redisClient   RedisClient
	pubSubManager PubSubManager
	wsManager     *websocket.Manager
	languages     []string
}

type Match struct {
	ID        string     `json:"match_id"`
	User1     QueueEntry `json:"user1"`
	User2     QueueEntry `json:"user2"`
	CreatedAt time.Time  `json:"created_at"`
}

func NewMatchingService(redisClient RedisClient, pubSubManager PubSubManager, wsManager *websocket.Manager, languages []string) *MatchingService {
	return &MatchingService{
		redisClient:   redisClient,
		pubSubManager: pubSubManager,
		wsManager:     wsManager,
		languages:     languages,
	}
}

func (s *MatchingService) Start(ctx context.Context) {
	for _, language := range s.languages {
		go s.listenToLanguageChannel(ctx, language)
	}
	log.Printf("Matching service started for %d languages", len(s.languages))
}

func (s *MatchingService) listenToLanguageChannel(ctx context.Context, language string) {
	pubsub := s.pubSubManager.SubscribeToLanguageChannel(ctx, language)
	defer pubsub.Close()

	log.Printf("Listening to channel for language: %s", language)

	ch := pubsub.Channel()
	for msg := range ch {
		var newUser QueueEntry
		if err := json.Unmarshal([]byte(msg.Payload), &newUser); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		log.Printf("New user in %s channel: %s (native: %s, practice: %s)",
			language, newUser.UserID, newUser.NativeLanguage, newUser.PracticeLanguage)

		match, err := s.findMatch(ctx, newUser)
		if err != nil {
			log.Printf("Error finding match: %v", err)
			continue
		}

		if match != nil {
			log.Printf("Match found! %s <-> %s", match.User1.UserID, match.User2.UserID)
			if err := s.notifyMatch(match); err != nil {
				log.Printf("Error notifying match: %v", err)
			}
		}
	}
}

func (s *MatchingService) findMatch(ctx context.Context, newUser QueueEntry) (*Match, error) {
	queueKey := "queue:" + newUser.NativeLanguage

	queueLength, err := s.redisClient.LLen(ctx, queueKey).Result()
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < queueLength; i++ {
		entryJSON, err := s.redisClient.LIndex(ctx, queueKey, i).Result()
		if err != nil {
			continue
		}

		var candidateUser QueueEntry
		if err := json.Unmarshal([]byte(entryJSON), &candidateUser); err != nil {
			continue
		}

		if s.isCompatibleMatch(newUser, candidateUser) {
			if err := s.removeFromQueue(ctx, candidateUser); err != nil {
				log.Printf("Error removing user from queue: %v", err)
				continue
			}

			matchID := fmt.Sprintf("match_%d", time.Now().Unix())
			return &Match{
				ID:        matchID,
				User1:     newUser,
				User2:     candidateUser,
				CreatedAt: time.Now(),
			}, nil
		}
	}

	return nil, nil
}

func (s *MatchingService) isCompatibleMatch(user1, user2 QueueEntry) bool {
	if user1.UserID == user2.UserID {
		return false
	}

	return user1.NativeLanguage == user2.PracticeLanguage &&
		user1.PracticeLanguage == user2.NativeLanguage
}

func (s *MatchingService) removeFromQueue(ctx context.Context, user QueueEntry) error {
	queueKey := "queue:" + user.PracticeLanguage
	userJSON, err := json.Marshal(user)
	if err != nil {
		return err
	}

	return s.redisClient.LRem(ctx, queueKey, 1, userJSON).Err()
}

func (s *MatchingService) notifyMatch(match *Match) error {
	notification1 := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.User2.UserID,
		Language1: match.User1.NativeLanguage,
		Language2: match.User1.PracticeLanguage,
		Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.User1.PracticeLanguage, match.User2.UserID),
	}

	notification2 := websocket.MatchNotification{
		MatchID:   match.ID,
		PartnerID: match.User1.UserID,
		Language1: match.User2.NativeLanguage,
		Language2: match.User2.PracticeLanguage,
		Message:   fmt.Sprintf("Match found! You'll practice %s with %s", match.User2.PracticeLanguage, match.User1.UserID),
	}

	if err := s.wsManager.NotifyMatch(match.User1.UserID, notification1); err != nil {
		log.Printf("Failed to notify user %s: %v", match.User1.UserID, err)
	}

	if err := s.wsManager.NotifyMatch(match.User2.UserID, notification2); err != nil {
		log.Printf("Failed to notify user %s: %v", match.User2.UserID, err)
	}

	return nil
}
