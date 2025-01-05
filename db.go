package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	// if the database is closed (we are shutting down nicely)
	sealed     bool
	connection *sql.DB
}

func LoadDatabase(file string) (db *Database, err error) {
	err = os.MkdirAll("data", 0750)
	if err != nil {
		return
	}

	conn, err := sql.Open("sqlite3", filepath.Join("data", file))
	db = &Database{
		connection: conn,
		sealed:     false,
	}

	if err == nil {
		err = db.setup()
	}

	return
}

func (db *Database) Close() {
	if db.sealed {
		return
	}
	log.Info().Msgf("Sealing database")
	db.sealed = true
	db.connection.Close()
}

// `videos` contains info about the videos (Duh)
//   - `url` - MUST BE a direct link (ie: https://youtube.com/watch/v?= OR https://youtube.com/clip/ ) (verified by yt-dlp)
// 			   for youtu.be it will be transformed to the youtube.com one.
//   - `type` - either clip or video
// 	 - `uploader` - the (video) uploader, not the one who submitted it
//   - `finalist` - boolean (1, 0) if we are a finalist
//	 - `banned` - wont show up at all
// `users` standard user storage
// 	 - `ratelimit` - ratelimit timeout, next time you can do votes
//	 - `ratelimit_failcount` - number of requests that marked as failed
//	 - `ratelimit_last_request` - time since last request
// `votes_preliminary` votes (a) user made, also tracks if they voted something repeatedly
//	 - `vote_object` the json array object the user will give +1.
//					 technically the user CAN give +1 to all and that would nullify their votes (MUST be videos key)
// 					 populate this with the user's votes

// seconds/request
const ratelimit_per_sec = 1 * 10
const ratelimit_maxfail = 5

const schema = `
	CREATE TABLE IF NOT EXISTS videos (
		id INTEGER PRIMARY KEY,
		url TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		uploader TEXT NOT NULL,
		finalist INTEGER NOT NULL DEFAULT 0,
		banned INTEGER NOT NULL DEFAULT 0,
		date_submitted INTEGER NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY,
		banned INTEGER NOT NULL DEFAULT 0,
		ip TEXT UNIQUE NOT NULL,
		ratelimit INTEGER DEFAULT 0,
		ratelimit_failcount INTEGER DEFAULT 0,
		ratelimit_last_request INTEGER DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS votes_preliminary (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		vote_object jsonb NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS votes_finalist (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		voted_clip INTEGER NOT NULL,
		FOREIGN KEY(voted_clip) REFERENCES videos(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
`

var funny_moments = []string{
	"https://www.youtube.com/watch?v=oxEUk5c1iGU",
	"https://www.youtube.com/watch?v=TFNYbCGCIaw",
	"https://www.youtube.com/watch?v=N0qzSv9c0IY",
	"https://www.youtube.com/watch?v=sZN5yJqDaYI",
	"https://www.youtube.com/watch?v=TQllQlElpz8",
	"https://www.youtube.com/watch?v=WRRC-Iw_OPg",
	"https://www.youtube.com/watch?v=72eGw4H2Ka8",
	"https://www.youtube.com/watch?v=4LilrtDfLP0",
	"https://www.youtube.com/watch?v=uSlB4eznXoA",
	"https://www.youtube.com/watch?v=i9bYnBb42oY",
	"https://www.youtube.com/watch?v=lNfCvZl3sKw",
	"https://www.youtube.com/watch?v=nz_BY7X44kc",
	"https://www.youtube.com/watch?v=xrziHnudx3g",
	"https://www.youtube.com/watch?v=2WNrx2jq184",
	"https://www.youtube.com/watch?v=el0jsvcOSTg",
	"https://www.youtube.com/watch?v=4hpbK7V146A",
	"https://www.youtube.com/watch?v=Ta_-UPND0_M",
	"https://www.youtube.com/watch?v=JgJUbmGDc6k",
	"https://www.youtube.com/watch?v=ttArr90NvWo",
	"https://www.youtube.com/watch?v=mIpnpYsl-VY",
	"https://www.youtube.com/watch?v=4LilrtDfLP0",
	"https://www.youtube.com/watch?v=0pnwE_Oy5WI",
}

// Setup the database.
// Ran every time we load the database.
func (db *Database) setup() (err error) {
	// Transaction so we can undo if we error
	tran, err := db.connection.Begin()
	if err != nil {
		return
	}

	_, err = db.connection.Exec(schema)
	if err != nil {
		tran.Rollback()
		return
	}

	// TODO test code delete when on deployment
	log.Print("DEVEL: adding dummy data")
	for _, funny := range funny_moments {
		db.AddVideo(funny)
	}
	log.Print("DEVEL: adding dummy data again, but this shouldnt cause double adding")
	for _, funny := range funny_moments {
		db.AddVideo(funny)
	}
	log.Print("DEVEL: testing get video (should not error)")
	for _, funny := range funny_moments {
		if db.GetVideo(funny) == 0 {
			log.Print("DEVEL: something broke on the db machine")
		}
	}
	log.Print("DEVEL: creating dummy user")
	u1, _ := db.GetUser("192.168.1.1")
	u2, _ := db.GetUser("192.168.1.1")
	if u1.id != u2.id {
		log.Print("DEVEL: user does NOT match even if ip is the same!")
	}
	u3, _ := db.GetUser("192.168.1.2")
	log.Print("DEVEL: doing votes")
	log.Print("DEVEL: next one should fail")
	db.VoteVideo("https://www.youtube.com/watch?v=fakevideo", u3)
	log.Print("DEVEL: next one should save votes")
	db.VoteVideo("https://www.youtube.com/watch?v=oxEUk5c1iGU", u3)
	db.VoteVideo("https://www.youtube.com/watch?v=TQllQlElpz8", u3)
	db.VoteVideo("https://www.youtube.com/watch?v=TQllQlElpz8", u3)
	log.Print("DEVEL: give me my votes ", db.MyVotes(u3))

	log.Print("db primed")
	// Commit transaction
	return tran.Commit()
}

// based on fail2ban
// do 5 bad request then we ban u for 20 sec
// you MUST call this on your http shenanigans
func (db *Database) ProcessRatelimit(user User) (shouldProcess bool, ratelim int) {
	var rfail int
	var rlim_lastreq int

	// probably should just have this be in memory instead of sqlite?
	err := db.connection.QueryRow(`
		SELECT ratelimit_failcount, ratelimit_last_request, ratelimit
		FROM users WHERE id = ?`, user.id).Scan(&rfail, &rlim_lastreq, &ratelim)

	// do not move me up the query
	var thyme = time.Now().UTC().UnixMilli()
	if err != nil || int64(ratelim) < thyme {
		return
	}

	shouldProcess = true
	if (int64(rlim_lastreq) - thyme) > ratelimit_per_sec {
		if rfail > ratelimit_maxfail {
			// BANNED
			db.connection.Exec(`UPDATE ratelimit = ?, ratelimit_failcount = 0, ratelimit_last_request = CURRENT_TIMESTAMP WHERE id = ?`, thyme+int64(20*time.Second), user.id)
			shouldProcess = false
			return
		}
		rfail += 1
	} else {
		rfail = max(0, rfail-1)
	}

	db.connection.Exec(`UPDATE ratelimit_last_request = CURRENT_TIMESTAMP ratelimit_failcount = ? WHERE id = ?`, rfail, user.id)
	return
}

// Add a video to the db. does not perform user verification
func (db *Database) AddVideo(url string) {
	if db.sealed {
		return
	}
	// no. even clips cut off at 80~ with the meta stuff included
	// should kill bogus stuff nicely
	url = strings.TrimSpace(url)
	url = url[:min(len(url), 100)]

	match, _ := regexp.Compile(`^https:\/\/(?:(?:www\.)?youtube\.com\/(?:watch\?v=|clip\/)|youtu\.be\/)`)
	if !match.MatchString(url) {
		log.Print("Invalid url string: ", url)
		return
	}

	var id = db.GetVideo(url)
	if id != 0 {
		return
	}

	// log.Print("AddVideo added")
	db.connection.Exec(`INSERT INTO videos (url, type, uploader) VALUES (?, ?, ?)`,
		url, "video", "not shounic")
}

func (db *Database) GetVideo(url string) (id int) {
	if db.sealed {
		return
	}
	url = strings.TrimSpace(url)
	url = url[:min(len(url), 100)]

	db.connection.QueryRow(`SELECT id FROM videos WHERE url = ?`, url).Scan(&id)
	return
}

// Vote a video to the db. does not perform user verification
func (db *Database) VoteVideo(url string, user User) {
	if db.sealed {
		return
	}
	var videoID = db.GetVideo(url)
	if videoID == 0 {
		log.Print("VoteVideo video id does not exist")
		return
	}

	var voteObject VoteObject
	var id int
	err := db.connection.QueryRow(`SELECT id, vote_object FROM votes_preliminary WHERE user_id = ?`, user.id).Scan(&id, &voteObject)
	if err == sql.ErrNoRows {
		// new voter
		voteObject = VoteObject{}
		voteObject[videoID] = true
		db.connection.Exec(`INSERT INTO votes_preliminary (user_id, vote_object) VALUES (?, ?)`,
			user.id, voteObject)
		log.Print("VoteVideo creating new vote stub")
		return
	} else if err != nil {
		// generic unspecified error
		log.Print("VoteVideo query errored", err)
		return
	}
	if voteObject[videoID] {
		log.Print("VoteVideo duplicate vote ", videoID)
		return
	}
	// regular user

	log.Print("VoteVideo user ", user.id, " voted ", videoID)
	voteObject[videoID] = true
	db.connection.Exec(`UPDATE votes_preliminary SET vote_object = ? WHERE id = ?`,
		voteObject, id)
}

// Return url to user
func (db *Database) MyVotes(user User) (videoUrls []string) {
	if db.sealed {
		return
	}
	var voteObject VoteObject
	err := db.connection.QueryRow(`SELECT vote_object FROM votes_preliminary WHERE user_id = ?`, user.id).Scan(&voteObject)
	if err != nil {
		log.Print("MyVotes query errored: ", err)
		return
	}
	for id, v := range voteObject {
		if !v {
			continue
		}
		var url string
		db.connection.QueryRow(`SELECT url FROM videos WHERE id = ?`, id).Scan(&url)
		videoUrls = append(videoUrls, url)
	}

	return
}

// Vote a video to the db (finalist). does not perform user verification
// TODO untested
func (db *Database) VoteFinalist(url string, user User) {
	var videoID = db.GetVideo(url)

	var id int
	err := db.connection.QueryRow(`SELECT id FROM votes_finalist WHERE user_id = ?`, user.id).Scan(&id)
	// bogus vote prolly (norows)
	if err == sql.ErrNoRows {
		// new voter
		db.connection.Exec(`INSERT INTO votes_finalist (user_id, voted_clip) VALUES (?, ?)`,
			user.id, videoID)
		return
	} else if err != nil {
		// generic unspecified error
		log.Print("VoteFinalist query errored", err)
		return
	}

	db.connection.Exec(`UPDATE votes_finalist SET voted_clip = ? WHERE user_id = ?`,
		id, videoID)
}

func (db *Database) GetUser(remoteAddr string) (user User, err error) {
	if db.sealed {
		err = os.ErrClosed
		return
	}
	user.ip = remoteAddr

	// Get user from database
	var id int
	err = db.connection.QueryRow("SELECT id FROM users WHERE ip=?", user.ip).Scan(&id)

	if err != nil && err != sql.ErrNoRows {
		log.Print("GetUser query user errored: ", err)
		return
	}
	if err == sql.ErrNoRows {
		// Add user if they do not already exist.
		err = db.connection.QueryRow("INSERT INTO users (ip) VALUES (?) RETURNING id", user.ip).Scan(&id)
		if err != nil {
			log.Print("GetUser set user errored: ", err)
			return
		}
	}
	user.id = uint(id)
	return
}

// Get the next vote for a user
// If a vote already exists, it will be deleted.
// If there are < 2 options, `vote` will be nil
func (db *Database) GetNextVoteForUser(user User) (vote *VoteOptions, err error) {
	a, b, err := db.findNextPair(user)
	if a == "" || b == "" || err != nil {
		// Return nil vote, we don't have enough
		// voting options for this user
		return
	}

	// Exec is 10x-100x slower for some reason.
	// Query has issues committing inserts
	// Locking issue?
	vote = &VoteOptions{time.Now(), a, b}
	_, err = db.connection.Exec(
		"INSERT OR REPLACE INTO active_votes VALUES (?, ?, ?, ?)",
		user.id,
		time.Now().UnixMilli(),
		a,
		b,
	)
	return
}

// Get new vote options for the user
// Empty a or b strings means not enough available voting options
func (db *Database) findNextPair(user User) (a string, b string, err error) {
	row, err := db.connection.Query(
		"SELECT url FROM videos WHERE url NOT IN (SELECT video_url FROM votes WHERE user_id = ?) ORDER BY random() LIMIT 2",
		user.id,
	)
	if err != nil {
		return
	}
	defer row.Close()

	if !row.Next() {
		// 0 videos available
		return
	}

	err = row.Scan(&a)
	if err != nil || !row.Next() {
		return
	}

	err = row.Scan(&b)
	return
}

func (db *Database) GetCurrentVotingOptionsForUser(user User) (vote *VoteOptions, err error) {
	row, err := db.connection.Query(
		"SELECT start_time, a, b FROM active_votes WHERE user_id = ?",
		user.id,
	)
	if err != nil {
		return
	}
	defer row.Close()

	if !row.Next() {
		// User has no vote options, returning nil
		return
	}

	var startTime int64
	vote = &VoteOptions{}
	err = row.Scan(
		&startTime,
		&vote.A,
		&vote.B,
	)

	vote.startTime = time.Unix(startTime, 0)

	return
}

func (db *Database) SubmitUserVote(user User, choice string) (err error) {
	vote, err := db.GetCurrentVotingOptionsForUser(user)
	if err != nil || vote == nil {
		// If the user has no options, we'll do nothing
		return
	}

	// TODO scale min time to video length
	// 	?	minTime := max(min(a.length, b.length) / 2, 90 * time.seconds)
	// if vote.startTime.Add(30 * time.Second).After(time.Now()) {
	// 	// User voting too fast, ignore vote
	// 	return fmt.Errorf("too fast")
	// }

	// TODO limit max time? 12hours?

	if choice != vote.A && choice != vote.B {
		fmt.Println("Invalid choice")
		return
	}

	// TODO only supports one round of votes
	_, err = db.connection.Exec(
		"DELETE FROM active_votes WHERE user_id = ?;"+
			"INSERT INTO votes VALUES (?, ?, 1);",
		user.id,
		user.id,
		choice,
	)
	return
}

func (db *Database) TallyVotes() (count map[string]int, err error) {
	count = make(map[string]int)

	// Populate the map
	row, err := db.connection.Query("SELECT url FROM videos")
	if err != nil {
		return
	}
	defer row.Close()
	for row.Next() {
		var url string
		err = row.Scan(&url)
		if err != nil {
			return
		}
		count[url] = 0
	}

	// Count the results
	row2, err := db.connection.Query("SELECT video_url, score FROM votes")
	if err != nil {
		return
	}
	defer row2.Close()
	for row2.Next() {
		var url string
		var score int
		err = row2.Scan(&url, &score)
		if err != nil {
			return
		}
		count[url] += score
	}

	return
}
