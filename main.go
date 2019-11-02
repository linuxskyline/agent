package main

import (
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	skyline "github.com/linuxskyline/goskyline"
	agent "github.com/linuxskyline/goskyline/agent"
	log "github.com/sirupsen/logrus"
)

func getBaseUrl() *url.URL {
	environmentVariable, exists := os.LookupEnv("API_BASE_URL")
	if !exists {
		log.WithFields(log.Fields{
			"stage": "initialization",
		}).Fatal("No api base url provided. Please set API_BASE_URL environment variable to something.")
	}

	baseUrl, err := url.Parse(environmentVariable)
	if err != nil {
		log.WithFields(log.Fields{
			"stage":        "initialization",
			"provided_url": os.Getenv("API_BASE_URL"),
		}).Fatal("Error parsing provided api base url.")
	}

	return baseUrl
}

func getToken() string {
	hostToken, exists := os.LookupEnv("API_HOST_TOKEN")
	if !exists {
		log.WithFields(log.Fields{
			"stage": "initialization",
		}).Fatal("No host token provided. Please set API_HOST_TOKEN environment variable to something.")
	}

	return hostToken
}

func updateListContains(list []*skyline.Update, item *skyline.Update) bool {
	for _, listItem := range list {
		if listItem.PackageName == item.PackageName {
			return true
		}
	}

	return false
}

func main() {
	client := agent.NewClient(getBaseUrl(), getToken())

	for _ = range time.Tick(5 * time.Second) {
		syncUpdates(client)
	}
}

func getAvailableUpdates() []*skyline.Update {
	updates := []*skyline.Update{}

	out, err := exec.Command("apt-get", "--just-print", "upgrade").Output()
	if err != nil {
		log.Fatal(err)
	}

	lines := Filter(strings.Split(string(out), "\n"), isPackageInstall)

	for _, line := range lines {
		packageInfo := parsePackageLine(line)
		updates = append(updates, &packageInfo)
	}

	return updates
}

func createUpdates(client *agent.Client, updates []*skyline.Update) {
	for _, update := range updates {
		client.CreateUpdate(*update)

		log.WithFields(log.Fields{
			"stage":          "newupdatepost",
			"packageName":    update.PackageName,
			"currentVersion": update.CurrentVersion,
			"nextVersion":    update.NewVersion,
			"security":       update.Security,
		}).Trace("Updated available update on server.")
	}
}

func pruneUpdates(client *agent.Client, updates []*skyline.Update) {
	newUpdates, err := client.GetUpdates()
	if err != nil {
		log.WithFields(log.Fields{
			"cause": err,
		}).Error("Failed to get list of existing updates from server")
	}

	for _, update := range newUpdates {
		if !updateListContains(updates, update) {
			log.WithFields(log.Fields{
				"stage":       "cleaningserverupdates",
				"packageName": update.PackageName,
			}).Info("Deleting update from server")
			client.DeleteUpdate(update)
		}
	}
}

func syncUpdates(client *agent.Client) {
	updates := getAvailableUpdates()
	log.Info("Posting new updates to the server")
	createUpdates(client, updates)
	log.Info("Pruning updates from the server")
	pruneUpdates(client, updates)
}

func isPackageInstall(line string) bool {
	return strings.HasPrefix(line, "Inst")
}

func parsePackageLine(line string) skyline.Update {
	state := 1

	toReturn := skyline.Update{}

	for _, character := range line {
		switch state {
		case 1:
			if character == ' ' {
				state = 2
				continue
			}
			continue
		case 2:
			if character == ' ' {
				state = 3
				continue
			}

			toReturn.PackageName = toReturn.PackageName + string(character)
		case 3:
			if character == '[' {
				state = 4
				continue
			}
		case 4:
			if character == ']' {
				state = 5
				continue
			}
			toReturn.CurrentVersion = toReturn.CurrentVersion + string(character)
		case 5:
			if character == '(' {
				state = 6
				continue
			}
		case 6:
			if character == ' ' {
				state = 7
				continue
			}

			toReturn.NewVersion = toReturn.NewVersion + string(character)
		case 7:
			if character == ')' {
				state = 8
				continue
			}

			toReturn.Repository = toReturn.Repository + string(character)
		default:
			break
		}
	}

	if strings.Contains(toReturn.Repository, "-security") {
		toReturn.Security = true
	}

	return toReturn
}

// Returns a new slice containing all strings in the
// slice that satisfy the predicate `f`.
func Filter(vs []string, f func(string) bool) []string {
	vsf := make([]string, 0)
	for _, v := range vs {
		if f(v) && len(v) > 7 {
			vsf = append(vsf, v)
		}
	}
	return vsf
}
