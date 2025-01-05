package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var envDBFile = getEnvOrDefault("CLIPS_DB", "votes.db?_mutex=full&_journal_mode=wal")
var envBindAddr = getEnvOrDefault("CLIPS_BIND", ":8081")
var envBehindProxy = os.Getenv("CLIPS_BEHIND_PROXY")

var database *Database

var votingDeadlineUnix int64 = time.Date(2025, time.January, 15, 24, 30, 50, 0, time.UTC).UnixMilli()

func main() {
	setupLogger()
	log.Info().Msgf("Loading database from %s", envDBFile)
	database, err := LoadDatabase(envDBFile)
	if err != nil {
		panic(err)
	}

	log.Info().Msgf("Starting http server on %s", envBindAddr)
	serveMux := CustomMux{http.NewServeMux()}
	initRoutes(serveMux)
	server := &http.Server{Addr: envBindAddr, Handler: serveMux}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			// err can happen if same port used etc...
			database.Close()
			panic(err)
		}
	}()

	// this guy will lock up process waiting for the signal
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGSEGV)
	sig := <-sigc
	switch sig {
	case os.Interrupt:
	case syscall.SIGQUIT:
	case syscall.SIGTERM:
	case syscall.SIGSEGV: // gnome sys manager sends this on 'end process'
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// triggers shutdown, also exits sig loop (if no err)
		if err := server.Shutdown(ctx); err != nil {
			panic(err)
		}
	}

	database.Close()
	log.Info().Msgf("Program exiting!")
	os.Exit(0)
}

func setupLogger() {
	logDir := "logs"
	_ = os.MkdirAll(logDir, os.ModePerm)

	// Create log file for the current session.
	formattedDate := time.Now().UTC().Format(time.DateTime)
	logFile := logDir + "/" + formattedDate + ".log"
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		panic("unable to open log file")
	}

	// Attach log output to the log file and an application terminal.
	fileWriter := zerolog.ConsoleWriter{
		Out:        file,
		TimeFormat: time.DateTime,
		NoColor:    true,
	}
	consoleWrite := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.DateTime,
	}
	multi := zerolog.MultiLevelWriter(fileWriter, consoleWrite)
	logger := zerolog.New(multi).With().Caller().Timestamp().Logger()
	log.Logger = logger
}

func getEnvOrDefault(key string, defValue string) (value string) {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defValue
	}
	return
}
