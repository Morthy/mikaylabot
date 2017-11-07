package main

import (
	"fmt"
	"flag"
	"database/sql"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"syscall"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var (
	Token string
	Database *sql.DB
)

func init() {
	flag.StringVar(&Token, "t", "", "Token")
	flag.Parse()
}

func main() {
	var err error
	Database, err = sql.Open("sqlite3", "./db.db")

	if err != nil {
		fmt.Println("Could not open db", err)
		return
	}

	Database.Exec("CREATE TABLE IF NOT EXISTS commands (team_id varchar(64), command VARCHAR(64), message VARCHAR(255))")
	Database.Exec("CREATE UNIQUE INDEX if not exists team_command ON commands (team_id, command)")

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func listCommands(s *discordgo.Session, channelID string, teamID string) {
	rows, _ := Database.Query("SELECT command, message FROM commands WHERE team_id=? ORDER BY command ASC", teamID)

	var command string
	var message string

	for rows.Next() {
		rows.Scan(&command, &message)
		fmt.Println(command, message)
		s.ChannelMessageSend(channelID, fmt.Sprintf("!%s: %s", command, message))
	}
}

func addCommand(s *discordgo.Session, channelID string, teamID string, command string, text string) {
	Database.Exec("REPLACE INTO commands VALUES (?, ?, ?)", teamID, command, text)
	s.ChannelMessageSend(channelID, fmt.Sprintf("Successfully created/updated command !%s", command))
}

func delCommand(s *discordgo.Session, channelID string, teamID string, command string) {
	result, _ := Database.Exec("DELETE FROM commands WHERE team_id=? AND command=?", teamID, command)
	rowsAffected, _ := result.RowsAffected()

	if rowsAffected > 0 {
		s.ChannelMessageSend(channelID, fmt.Sprintf("Deleted command !%s", command))
	}	else {
		s.ChannelMessageSend(channelID, fmt.Sprintf("Couldn't delete command because it didn't exist: !%s", command))
	}
}


// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, "!") {
		return
	}

	command := strings.TrimPrefix(m.Content, "!")
	parts := strings.Split(command, " ")
	command = parts[0]

	channel, err := s.State.Channel(m.ChannelID)

	if (err != nil) {
		log.Println("Error getting channel", err.Error())
		return
	}

	guild, err := s.State.Guild(channel.GuildID)

	if (err != nil) {
		log.Println("Error getting guild", err.Error())
		return
	}

	fmt.Println(command)

	if command == "echos" && isMessageFromMod(s, channel, guild, m.Author.ID)  {
		if len(parts) < 2 {
			return
		}

		subcommand := parts[1]

		fmt.Println(subcommand)

		if subcommand == "list" {
			listCommands(s, m.ChannelID, guild.ID)
		} else if (subcommand == "add" && len(parts) >= 4) {
			newCommandName := parts[2]
			message := strings.Join(parts[3:], " ")
			addCommand(s, m.ChannelID, guild.ID, newCommandName, message)
		} else if (subcommand == "del" && len(parts) == 3) {
			delCommandName := parts[2]
			delCommand(s, m.ChannelID, guild.ID, delCommandName)
		}

		return
	}


	var message string
	err = Database.QueryRow("SELECT message FROM commands WHERE team_id=? AND command=?", guild.ID, command).Scan(&message)

	switch {
		case err == sql.ErrNoRows:
			return
		case err != nil:
			log.Fatal(err)
			return
	}

	fmt.Println(message)

	s.ChannelMessageSend(m.ChannelID, message)
	return;
}

func isMessageFromMod(s *discordgo.Session, channel *discordgo.Channel, guild *discordgo.Guild, userID string) bool {
	if (guild.OwnerID == userID) {
		return true
	}

	member, err := s.GuildMember(channel.GuildID, userID)

	if (err != nil) {
		log.Println("Error getting member", err.Error)
		return false
	}

	permissions := 0

	for _, role := range guild.Roles {
		for _, roleID := range member.Roles {
			if role.ID == roleID {
				permissions = permissions | role.Permissions
			}
		}
	}

	if permissions & discordgo.PermissionManageServer != 0 {
		return true;
	}

	return false;
}