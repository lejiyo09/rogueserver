/*
	Copyright (C) 2024  Pagefault Games

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/gob"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/pagefaultgames/rogueserver/api"
	"github.com/pagefaultgames/rogueserver/api/account"
	"github.com/pagefaultgames/rogueserver/db"
)

func main() {
	// env stuff
	debug, _ := strconv.ParseBool(os.Getenv("debug"))

	proto := getEnv("proto", "tcp")
	addr := resolveAddr()
	tlscert := getEnv("tlscert", "")
	tlskey := getEnv("tlskey", "")

	dbuser := getEnv("dbuser", "pokerogue")
	dbpass := getEnv("dbpass", "pokerogue")
	dbproto := getEnv("dbproto", "tcp")
	dbaddr := getEnv("dbaddr", "localhost")
	dbname := getEnv("dbname", "pokeroguedb")

	discordclientid := getEnv("discordclientid", "")
	discordsecretid := getEnv("discordsecretid", "")

	googleclientid := getEnv("googleclientid", "")
	googlesecretid := getEnv("googlesecretid", "")

	callbackurl := getEnv("callbackurl", "http://localhost:8001/")

	gameurl := getEnv("gameurl", "https://pokerogue.net")

	discordbottoken := getEnv("discordbottoken", "")
	discordguildid := getEnv("discordguildid", "")

	account.GameURL = gameurl

	account.DiscordClientID = discordclientid
	account.DiscordClientSecret = discordsecretid
	account.DiscordCallbackURL = callbackurl + "/auth/discord/callback"

	account.GoogleClientID = googleclientid
	account.GoogleClientSecret = googlesecretid
	account.GoogleCallbackURL = callbackurl + "/auth/google/callback"
	account.DiscordSession, _ = discordgo.New("Bot " + discordbottoken)
	account.DiscordGuildID = discordguildid

	// register gob types
	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})

	// get database connection
	err := db.Init(dbuser, dbpass, dbproto, dbaddr, dbname)
	if err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}

	// create listener
	listener, err := createListener(proto, addr)
	if err != nil {
		log.Fatalf("failed to create net listener: %s", err)
	}

	mux := http.NewServeMux()

	// init api
	if err := api.Init(mux); err != nil {
		log.Fatal(err)
	}

	// start web server
	handler := prodHandler(mux, gameurl)
	if debug {
		handler = debugHandler(mux)
	}

	if tlscert == "" {
		err = http.Serve(listener, handler)
	} else {
		err = http.ServeTLS(listener, handler, tlscert, tlskey)
	}
	if err != nil {
		log.Fatalf("failed to create http server or server errored: %s", err)
	}
}

func createListener(proto, addr string) (net.Listener, error) {
	if proto == "unix" {
		os.Remove(addr)
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		return nil, err
	}

	if proto == "unix" {
		if err := os.Chmod(addr, 0777); err != nil {
			listener.Close()
			return nil, err
		}
	}

	return listener, nil
}

// prodHandler serves the API with CORS headers scoped to the game client(s).
// clienturl is normally a single origin (unchanged historical behavior: that
// exact value is always sent back, regardless of the request's own Origin).
// It may also be a comma-separated list of origins - e.g. to temporarily
// allow a local file:// test page (Origin: null) alongside the real game
// client - in which case the request's Origin is reflected back only when
// it's one of the configured origins, so browsers only treat the response
// as readable by origins the operator explicitly allowed.
func prodHandler(router *http.ServeMux, clienturl string) http.Handler {
	allowedOrigins := strings.Split(clienturl, ",")
	for i := range allowedOrigins {
		allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin(allowedOrigins, r.Header.Get("Origin")))

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		router.ServeHTTP(w, r)
	})
}

// allowedOrigin picks the Access-Control-Allow-Origin value for a request.
// With a single configured origin (today's default for every existing
// deployment), that origin is always returned, matching prior behavior
// exactly. With more than one configured, the request's own Origin is
// reflected back only if it's in the allow-list; otherwise the first
// configured origin is returned (a browser will still reject reading the
// response for a mismatched origin, same as before this change existed).
func allowedOrigin(allowed []string, requestOrigin string) string {
	if len(allowed) == 1 {
		return allowed[0]
	}

	for _, origin := range allowed {
		if origin == requestOrigin {
			return origin
		}
	}

	return allowed[0]
}

func debugHandler(router *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		router.ServeHTTP(w, r)
	})
}

// resolveAddr determines the address to listen on. An explicit "addr"
// environment variable always wins. Otherwise, on platforms like Render
// that assign a listen port via the PORT environment variable and route
// external traffic to it, listen on 0.0.0.0 at that port - so deployments
// there don't need to set addr explicitly. When neither is set, this keeps
// the historical local/docker-compose default of 0.0.0.0:8001.
func resolveAddr() string {
	return getEnv("addr", "0.0.0.0:"+getEnv("PORT", "8001"))
}

func getEnv(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return defaultValue
}
