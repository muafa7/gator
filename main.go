package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"github.com/muafa7/gator/internal/config"
	"github.com/muafa7/gator/internal/database"
)

type state struct {
	db  *database.Queries
	cfg *config.Config
}

type commands struct {
	handlers map[string]func(*state, command) error
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.handlers[name] = f
}

func (c *commands) run(s *state, cmd command) error {
	handler, exist := c.handlers[cmd.name]
	if !exist {
		return errors.New("unknown command")
	}

	return handler(s, cmd)
}

type command struct {
	name string
	args []string
}

func middlewareLoggedIn(handler func(s *state, cmd command, user database.User) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		user, err := s.db.GetUser(context.Background(), s.cfg.CurrentUserName)
		if err != nil {
			return err
		}

		return handler(s, cmd, user)
	}
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("username not provided")
	}

	username := cmd.args[0]

	_, err := s.db.GetUser(context.Background(), username)
	if err != nil {
		return err
	}

	if err := s.cfg.SetUser(username); err != nil {
		return err
	}

	fmt.Printf("User set to %s\n", username)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("name not provided")
	}

	name := cmd.args[0]
	now := time.Now()

	user, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
	})
	if err != nil {
		return err
	}

	if err := s.cfg.SetUser(name); err != nil {
		return err
	}

	fmt.Printf("User %s was created\n", name)
	log.Printf("%+v\n", user)

	return nil
}

func handlerReset(s *state, cmd command) error {
	err := s.db.ResetUsers(context.Background())
	if err != nil {
		return err
	}

	fmt.Println("Reset success")

	return nil
}

func handlerGetUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return err
	}

	for _, user := range users {
		output := "* " + user.Name
		if user.Name == s.cfg.CurrentUserName {
			output += " (current)"
		}
		fmt.Println(output)
	}

	return nil
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) != 1 {
		return errors.New("duration not provided")
	}

	duration, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Collecting feeds every %s\n", duration)

	ticker := time.NewTicker(duration)
	for ; ; <-ticker.C {
		err = scrapeFeeds(s)
		if err != nil {
			fmt.Printf("Error %s when fetch\n", err)
		}
	}
}

func scrapeFeeds(s *state) error {
	ctx := context.Background()

	feed, err := s.db.GetNextFeedToFetch(ctx)
	if err != nil {
		return err
	}

	err = s.db.MarkFeedFetched(ctx, feed.ID)
	if err != nil {
		return err
	}

	resp, err := fetchFeed(ctx, feed.Url)
	if err != nil {
		return err
	}

	fmt.Printf("Feed name: %s\n", feed.Name)
	for _, item := range resp.Channel.Item {
		title := sql.NullString{
			String: item.Title,
			Valid:  item.Title != "",
		}

		description := sql.NullString{
			String: item.Description,
			Valid:  item.Description != "",
		}

		var publishedAt sql.NullTime
		if item.PubDate != "" {
			t, err := time.Parse(time.RFC1123Z, item.PubDate)
			if err != nil {
				t, err = time.Parse(time.RFC1123, item.PubDate)
			}
			if err == nil {
				publishedAt = sql.NullTime{
					Time:  t,
					Valid: true,
				}
			}
		}

		_, err := s.db.CreatePost(ctx, database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Title:       title,
			Description: description,
			Url:         item.Link,
			PublishedAt: publishedAt,
			FeedID:      feed.ID,
		})
		if err != nil {
			// ignore duplicate URL errors
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
				continue
			}

			log.Printf("couldn't create post %q: %v", item.Title, err)
			continue
		}

		fmt.Printf("Saved post: %s\n", item.Title)
	}

	return nil
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return errors.New("name and url should be provided")
	}

	ctx := context.Background()
	name := cmd.args[0]
	url := cmd.args[1]
	now := time.Now()

	feed, err := s.db.CreateFeed(ctx, database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
		Url:       url,
		UserID:    user.ID,
	})
	if err != nil {
		return err
	}

	_, err = s.db.CreateFeedFollow(ctx, database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", feed)

	return nil
}

func handlerGetFeeds(s *state, cmd command) error {
	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		fmt.Printf("Name: %s\n", feed.Name)
		fmt.Printf("URL: %s\n", feed.Url)
		fmt.Printf("User: %s\n", feed.UserName)
		fmt.Println()
	}

	return nil
}

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("url not provided")
	}

	ctx := context.Background()
	url := cmd.args[0]

	feed, err := s.db.GetFeed(ctx, url)
	if err != nil {
		return err
	}

	now := time.Now()

	feedFollow, err := s.db.CreateFeedFollow(ctx, database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created follow for %s and %s\n", feedFollow.FeedName, feedFollow.UserName)

	return nil
}

func handlerFollowing(s *state, cmd command, user database.User) error {
	follows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}

	for _, follow := range follows {
		fmt.Printf("* %s\n", follow.FeedName)
	}

	return nil
}

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("url not provided")
	}

	ctx := context.Background()
	url := cmd.args[0]

	feed, err := s.db.GetFeed(ctx, url)
	if err != nil {
		return err
	}

	ids := database.DeleteFeedFollowsParams{
		FeedID: feed.ID,
		UserID: user.ID,
	}

	err = s.db.DeleteFeedFollows(ctx, ids)
	if err != nil {
		return err
	}

	fmt.Printf("Success unfollow %s\n", feed.Name)

	return nil
}

func handlerBrowse(s *state, cmd command, user database.User) error {
	limit := int32(2)

	if len(cmd.args) > 1 {
		return errors.New("too many arguments")
	}

	if len(cmd.args) == 1 {
		n, err := strconv.Atoi(cmd.args[0])
		if err != nil {
			return errors.New("limit must be a number")
		}
		if n <= 0 {
			return errors.New("limit must be greater than 0")
		}
		limit = int32(n)
	}

	posts, err := s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
		UserID: user.ID,
		Limit:  limit,
	})
	if err != nil {
		return err
	}

	for _, post := range posts {
		fmt.Printf("Title: %s\n", post.Title.String)
		fmt.Printf("URL: %s\n", post.Url)

		if post.PublishedAt.Valid {
			fmt.Printf("Published: %s\n", post.PublishedAt.Time.Format(time.RFC1123))
		}

		if post.Description.Valid {
			fmt.Printf("Description: %s\n", post.Description.String)
		}

		fmt.Println()
	}

	return nil
}

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		log.Fatal(err)
	}

	dbQueries := database.New(db)

	s := state{
		db:  dbQueries,
		cfg: &cfg,
	}

	_ = s

	cmds := commands{
		handlers: make(map[string]func(*state, command) error),
	}

	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerGetUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", handlerGetFeeds)
	cmds.register("follow", middlewareLoggedIn(handlerFollow))
	cmds.register("following", middlewareLoggedIn(handlerFollowing))
	cmds.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	cmds.register("browse", middlewareLoggedIn(handlerBrowse))

	if len(os.Args) < 2 {
		log.Fatal("not enough arguments")
	}

	cmd := command{
		name: os.Args[1],
		args: os.Args[2:],
	}

	err = cmds.run(&s, cmd)
	if err != nil {
		log.Fatal(err)
	}
}
