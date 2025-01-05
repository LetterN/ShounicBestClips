package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

//go:embed www/*
var embedWWW embed.FS

func initRoutes(serveMux CustomMux) {
	serveMux.NewUserRoute("/vote/next", routeNextVote)
	serveMux.NewUserRoute("/vote/submit", routeSubmitVote)
	serveMux.NewUserRoute("/vote/deadline", routeSendDeadline)
	serveMux.NewUserRoute("/vote/totals", routeTotals)

	fs, err := fs.Sub(embedWWW, "www")
	if err != nil {
		panic(err)
	}

	serveMux.Handle("/", http.FileServerFS(fs))
}

// Middleware TODO
//		Rate limiting
//      Prevent voting after a cutoff time

func routeNextVote(w http.ResponseWriter, req *CustomRequest, user User) {
	w.Header().Add("Content-Type", "application/json; charset=utf-8")

	isLimited, limitTime := database.ProcessRatelimit(user)
	if isLimited {
		w.WriteHeader(429)
		w.Header().Add("x-ratelimit-reset", fmt.Sprint(limitTime))
		w.Write([]byte(`{"message": "Ratelimited"}`))
		return
	}

	if votingDeadlineUnix < time.Now().UTC().UnixMilli() {
		w.WriteHeader(403)
		w.Write([]byte(`{"message": "Voting is closed"}`))
		return
	}

	options, err := database.GetNextVoteForUser(user)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"message": "Failed to fetch from database"}`))
		// TODO log to Sentry
		fmt.Printf("Failed to get new votes for user %v \"%s\"\n", user, err)
		return
	}

	// User has completed their queue
	if options == nil {
		w.WriteHeader(200) // why are you sending no content even if there IS content, right theres
		w.Write([]byte(`{"message": "No more items to vote on!"}`))
		return
	}

	// Send new vote to client
	bytes, err := json.Marshal(options)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"message": "Failed to write JSON data"}`))
		// TODO log to Sentry
		fmt.Printf("Failed to write json data %v\n", options)
		return
	}

	w.Write(bytes)
}

func routeSubmitVote(w http.ResponseWriter, req *CustomRequest, user User) {
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	isLimited, limitTime := database.ProcessRatelimit(user)
	if isLimited {
		w.WriteHeader(429)
		w.Header().Add("x-ratelimit-reset", fmt.Sprint(limitTime))
		w.Write([]byte(`{"message": "Ratelimited"}`))
		return
	}

	if votingDeadlineUnix < time.Now().UTC().UnixMilli() {
		w.WriteHeader(403)
		w.Write([]byte(`{"message": "Voting is closed"}`))
		return
	}

	if err := req.ParseForm(); err != nil {
		w.WriteHeader(406)
		w.Write([]byte(`{"message": "Failed to parse form input"}`))
		return
	}

	choice := req.PostForm.Get("choice")
	if choice == "" {
		w.WriteHeader(400)
		w.Write([]byte(`{"message": "No choice given"}`))
		return
	}

	err := database.SubmitUserVote(user, choice)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"message": "Failed to communicate with database"}`))
		// TODO log to Sentry
		fmt.Printf("Failed to submit vote from %v of \"%s\": %v\n", user, choice, err)
		return
	}

	// Removing this and manually making another get request is easier than handling get request when I submit data
	// -myth
	//routeNextVote(w, req, user)
}

// this should be on the client
func routeSendDeadline(w http.ResponseWriter, req *CustomRequest, user User) {
	bytes, err := json.Marshal(map[string]int64{"deadline": votingDeadlineUnix})

	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Failed to prepare deadline."))
		fmt.Printf("Failed to write json data regarding deadline timestamp")
		return
	}

	w.Write(bytes)
}

// TODO /myVotes

func routeTotals(w http.ResponseWriter, req *CustomRequest, user User) {
	count, err := database.TallyVotes()
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Failed to count votes."))
		fmt.Println(err.Error())
		return
	}

	bytes, err := json.Marshal(count)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Failed to write Json."))
		fmt.Println(err.Error())
		return
	}

	w.Write(bytes)
}
