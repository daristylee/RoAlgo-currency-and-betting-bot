package main

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/togatoga/goforces"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	Token     string
	BotPrefix string

	config *configStruct
)

type configStruct struct {
	Token     string `json:"Token"`
	BotPrefix string `json:"BotPrefix"`
}

type Bet struct {
	author, winner, loser string
	win, sum              int64
}

type EventBet struct {
	author, player string
	win, low, high int64
}
type Result struct {
	points, standing int64
}

func ReadConfig() error {
	fmt.Println("Reading config file...")
	file, err := ioutil.ReadFile("./config.json")

	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	fmt.Println(string(file))

	err = json.Unmarshal(file, &config)

	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	Token = config.Token
	BotPrefix = config.BotPrefix

	return nil

}

var BotId string
var goBot *discordgo.Session
var balance = make(map[string]int64)
var bets = make(map[int64]*list.List)
var event bool = false
var event_betting bool = false
var event_rewarded bool = false
var event_points int64 = 0
var event_standings int64 = 0
var event_bets_points = make(map[string]*list.List)
var event_bets_standings = make(map[string]*list.List)
var event_results = make(map[string]Result)
var have_event_results = false

func Start() {
	goBot, err := discordgo.New("Bot " + config.Token)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	u, err := goBot.User("@me")

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	BotId = u.ID

	goBot.AddHandler(messageHandler)

	err = goBot.Open()

	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Println("Bot is running !")
}

const COTA_MAX = 50100
const COTA_MIN = 101
const COMISION = 5
const COMISION_EVENTS = 10

func limit_cota(cota int) int {
	if cota > COTA_MAX {
		cota = COTA_MAX
	} else if cota < COTA_MIN {
		cota = COTA_MIN
	}

	return cota
}

func cota(r1 int, r2 int) (int, int) {
	var c1 float64 = (math.Pow(10, -(float64)(r1-r2)/400)+1)*100 - 5
	var c2 float64 = (math.Pow(10, -(float64)(r2-r1)/400)+1)*100 - 5

	return limit_cota(int(c1)), limit_cota(int(c2))
}

func cota_points(low int64, high int64) int64 {
	var ret = event_points + 1
	if low != high {
		ret = event_points * 100 / (high - low)
	}

	return ret
}

func cota_standings(low int64, high int64) int64 {
	var ret = event_standings + 1
	if low != high {
		ret = event_standings * 100 / (high - low)
	}

	return ret
}

func add(argv []string, argc int) string {
	var mes = ""
	if argc != 3 {
		mes = "Invalid command"
	} else {
		var sum, err = strconv.ParseInt(argv[2], 10, 64)
		if err != nil {
			mes = "Invalid command"
		} else {
			mes = strconv.Itoa(int(sum)) + " were added to" + argv[1] + "'s balance"
			balance[argv[1]] += sum
		}
	}
	return mes
}

func help() string {
	var mes = ""
	mes = "```show user: shows user's balance" + "\n" + "show: shows your balance" +
		"\n" + "add user sum: adds sum to user's balance" + "\n" +
		"cota: calculates the quota between two users on codeforces" + "\n" +
		"bet cf: bet on someone that they are gonna beat another user on codeforces in standings" + "\n" + "```"
	return mes
}

func cota_cf(argv []string, argc int) string {
	var retval = ""
	var user1 = argv[2]
	var user2 = argv[3]

	ctx := context.Background()
	logger := log.New(os.Stderr, "*** ", log.LstdFlags)
	api, _ := goforces.NewClient(logger)

	var ratingChange1, err1 = api.GetUserRating(ctx, user1)
	var ratingChange2, err2 = api.GetUserRating(ctx, user2)
	if err1 == nil && err2 == nil {
		var rating1 = ratingChange1[len(ratingChange1)-1].NewRating
		var rating2 = ratingChange2[len(ratingChange2)-1].NewRating
		var cota1, cota2 = cota(rating1, rating2)

		retval = strconv.Itoa(cota1/100) + "." + strconv.Itoa(cota1/10%10) + strconv.Itoa(cota1%10) + "-" + strconv.Itoa(cota2/100) + "." + strconv.Itoa(cota2/10%10) + strconv.Itoa(cota2%10)
	} else {
		retval = "Invalid user(s)"
	}
	return retval
}

func show(user string) string {
	var mes = ""
	if user == "" {
		mes = "Invalid Command"
	} else {
		mes = user + "'s balance is " + strconv.Itoa(int(balance[user]))
	}
	return mes
}

func bet_cf(argv []string, user string) string {
	var retval = ""
	var user1 = argv[2]
	var user2 = argv[3]
	var sum, err1 = strconv.ParseInt(argv[3], 10, 64)
	var id, err2 = strconv.ParseInt(argv[4], 10, 64)

	if err1 != nil || err2 != nil {
		retval = "Invalid command"
	} else {
		ctx := context.Background()
		logger := log.New(os.Stderr, "*** ", log.LstdFlags)
		api, _ := goforces.NewClient(logger)
		contestList, _ := api.GetContestList(ctx, nil)
		var i = 0
		var length = len(contestList)
		for i < length && contestList[i].ID != id && !contestList[i].Finished() {
			i++
		}

		if !(contestList[i].ID == id && !contestList[i].Finished()) {
			retval = "The contest is invalid!"
		} else if balance[user] >= sum {

			balance[user] -= sum

			var ratingChange1, err1 = api.GetUserRating(ctx, user1)
			var ratingChange2, err2 = api.GetUserRating(ctx, user2)
			if err1 == nil && err2 == nil {
				var rating1 = ratingChange1[len(ratingChange1)-1].NewRating
				var rating2 = ratingChange2[len(ratingChange2)-1].NewRating
				var cota1, _ = cota(rating1, rating2)

				var win int64 = (sum - COMISION*sum/100) * int64(cota1) / 100

				bet := Bet{user, user1, user2, win, sum}
				bets[id].PushBack(bet)

				retval = "You bet " + strconv.Itoa(int(sum)) + " on " + user1 + " vs " + user2 + " in the Codforces contest: " + strconv.Itoa(int(id)) + " with a potentially win of " + strconv.Itoa(int(win))
			} else {
				retval = "Invalid user(s)"
			}
		} else {
			retval = "Insufficient funds, you have only " + strconv.Itoa(int(balance[user]))
		}
	}
	return retval
}

func event_start() string {
	event = true
	return "Event started"
}

func event_stop() string {
	event = false
	return "Event ended"
}

func event_betting_start() string {
	event_betting = true
	event_rewarded = false
	return "Event betting started"
}

func event_betting_stop() string {
	event_betting = false
	return "Event betting ended"
}

func event_cota_points(argv []string) string {
	var retval = ""
	var low, err1 = strconv.ParseInt(argv[4], 10, 64)
	var high, err2 = strconv.ParseInt(argv[5], 10, 64)

	if err1 == nil && err2 == nil {
		if low < high {
			var aux = low
			low = high
			high = aux
		}

		var cota = cota_points(low, high)
		retval = strconv.Itoa(int(cota/100)) + "." + strconv.Itoa(int(cota/10%10)) + strconv.Itoa(int(cota%10))
	} else {
		retval = "Invalid command"
	}

	return retval
}

func event_cota_standings(argv []string) string {
	var retval = ""
	var low, err1 = strconv.ParseInt(argv[4], 10, 64)
	var high, err2 = strconv.ParseInt(argv[5], 10, 64)

	if err1 != nil || err2 != nil {
		if low < high {
			var aux = low
			low = high
			high = aux
		}

		var cota = cota_standings(low, high)
		retval = strconv.Itoa(int(cota/100)) + "." + strconv.Itoa(int(cota/10%10)) + strconv.Itoa(int(cota%10))
	} else {
		retval = "Invalid command"
	}

	return retval
}

func event_bet_points(argv []string, user string) string {
	var retval = ""
	var low, err1 = strconv.ParseInt(argv[3], 10, 64)
	var high, err2 = strconv.ParseInt(argv[4], 10, 64)
	var sum, err3 = strconv.ParseInt(argv[5], 10, 64)
	var player = argv[6]

	if err1 == nil && err2 == nil && err3 == nil {

		if low < high {
			var aux = low
			low = high
			high = aux
		}

		if balance[user] >= sum {
			balance[user] -= sum

			var cota = cota_points(low, high)
			var win = (sum - COMISION_EVENTS*sum/100) * cota / 100
			event_bets_points[player].PushBack(EventBet{user, player, win, low, high})

			retval = "You bet " + strconv.Itoa(int(sum)) + " on " + player + " scoring between " + strconv.Itoa(int(low)) + " and" + strconv.Itoa(int(high))
		} else {
			retval = "Insufficient funds, you have only " + strconv.Itoa(int(balance[user]))
		}
	} else {
		retval = "Invalid command"
	}

	return retval
}
func event_bet_standings(argv []string, user string) string {
	var retval = ""
	var low, err1 = strconv.ParseInt(argv[3], 10, 64)
	var high, err2 = strconv.ParseInt(argv[4], 10, 64)
	var sum, err3 = strconv.ParseInt(argv[5], 10, 64)
	var player = argv[6]

	if err1 == nil && err2 == nil && err3 == nil {
		if low < high {
			var aux = low
			low = high
			high = aux
		}

		if balance[user] >= sum {
			balance[user] -= sum

			var cota = cota_points(low, high)
			var win = (sum - sum*COMISION_EVENTS/100) * cota / 100
			event_bets_standings[player].PushBack(EventBet{user, player, win, low, high})

			retval = "You bet " + strconv.Itoa(int(sum)) + " on " + player + " standing between " + strconv.Itoa(int(low)) + " and" + strconv.Itoa(int(high))
		} else {
			retval = "Insufficient funds, you have only " + strconv.Itoa(int(balance[user]))
		}
	} else {
		retval = "Invalid command"
	}

	return retval
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == BotId {
		return
	}

	var argv = strings.Split(m.Content, " ")
	var argc = len(argv)

	if argc == 1 && argv[0] == BotPrefix+"ping" {
		_, _ = s.ChannelMessageSend(m.ChannelID, "pong")
	} else if argc == 3 && argv[0] == BotPrefix+"add" {
		var msg = add(argv, argc)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"show" {
		var msg = show(m.Author.Username)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 2 && argv[0] == BotPrefix+"show" {
		var msg = show(argv[1])

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"help" {
		var msg = help()

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 4 && argv[0]+" "+argv[1] == BotPrefix+"cota cf" {
		var msg = cota_cf(argv, argc)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 6 && argv[0]+" "+argv[1] == BotPrefix+"bet cf" {
		var msg = bet_cf(argv, m.Author.Username)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"event start" {
		var msg = event_start()

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"event stop" {
		var msg = event_stop()

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"event betting start" {
		var msg = event_betting_start()

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if m.Content == BotPrefix+"event betting stop" {
		var msg = event_betting_stop()

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 5 && argv[0]+" "+argv[1]+" "+argv[3] == BotPrefix+"event cota points" {
		var msg = event_cota_points(argv)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 5 && argv[0]+" "+argv[1]+" "+argv[3] == BotPrefix+"event cota standings" {
		var msg = event_cota_standings(argv)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 7 && argv[0]+" "+argv[1]+" "+argv[2] == BotPrefix+"event bet points" {
		var msg = event_bet_points(argv, m.Author.Username)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if argc == 7 && argv[0]+" "+argv[1]+" "+argv[2] == BotPrefix+"event bet standings" {
		var msg = event_bet_standings(argv, m.Author.Username)

		_, _ = s.ChannelMessageSend(m.ChannelID, msg)
	} else if string(m.Content[0]) == BotPrefix {
		_, _ = s.ChannelMessageSend(m.ChannelID, "Invalid command")
	}

}

func main() {

	err := ReadConfig()

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	Start()
	fmt.Println("ok")

	const ONE_DAY = 24 * 60 * 60 * time.Second

	ctx := context.Background()
	logger := log.New(os.Stderr, "*** ", log.LstdFlags)
	api, _ := goforces.NewClient(logger)

	for true {
		fmt.Println("523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n523\n")

		time.Sleep(ONE_DAY)

		if have_event_results && !event_rewarded {
			for player, result := range event_results {
				for e := event_bets_points[player].Front(); e != nil; e = e.Next() {
					var bett = e.Value.(EventBet)
					if bett.low <= result.points && result.points <= bett.high {
						balance[bett.author] += bett.win
					}
				}

				for e := event_bets_standings[player].Front(); e != nil; e = e.Next() {
					var bett = e.Value.(EventBet)
					if bett.low <= result.standing && result.standing <= bett.high {
						balance[bett.author] += bett.win
					}
				}
			}
		}

		contestList, _ := api.GetContestList(ctx, nil)
		for key, value := range bets {
			i := 0
			for contestList[i].ID != key {
				i++
			}
			if contestList[i].Finished() {
				var standings, err = api.GetContestStandings(ctx, int(contestList[i].ID), nil)
				if err != nil {

					for e := value.Front(); e != nil; e = e.Next() {
						var bett = e.Value.(Bet)

						var i = 0
						var rankwinner int64 = -1
						var rankloser int64 = -1
						for rankwinner == -1 && rankloser == -1 && i < len(standings.Rows) {
							var j = 0
							for j < len(standings.Rows[i].Party.Members) {
								if standings.Rows[i].Party.Members[j].Handle == bett.winner {
									rankwinner = standings.Rows[i].Rank
								}
								if standings.Rows[i].Party.Members[j].Handle == bett.loser {
									rankloser = standings.Rows[i].Rank
								}
								j++
							}
							i++
						}

						if rankloser == -1 || rankwinner == -1 {
							balance[bett.author] += bett.sum
						} else if rankwinner < rankloser {
							balance[bett.author] += bett.win
						}
					}
				}
			}
		}

	}

	<-make(chan struct{})
	return
}
