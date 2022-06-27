/*
 * Copyright 2021 Skyscanner Limited.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * http://www.apache.org/licenses/LICENSE-2.0
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package update_prs

import (
	"errors"
	"fmt"
	"os"

	"github.com/skyscanner/turbolift/internal/campaign"
	"github.com/skyscanner/turbolift/internal/colors"
	"github.com/skyscanner/turbolift/internal/github"
	"github.com/skyscanner/turbolift/internal/logging"
	"github.com/skyscanner/turbolift/internal/prompt"
	"github.com/spf13/cobra"
)

var (
	gh github.GitHub = github.NewRealGitHub()
	p  prompt.Prompt = prompt.NewRealPrompt()
)

var (
	closeFlag bool
	yesFlag   bool
)

func NewUpdatePRsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-prs",
		Short: "update all PRs that have been generated by the campaign",
		Run:   run,
	}

	cmd.Flags().BoolVar(&closeFlag, "close", false, "Close all generated PRs")
	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skips the confirmation prompt")

	return cmd
}

// makes sure there is only one action activated
func onlyOne(args ...bool) bool {
	// simple counter
	b := map[bool]int{
		false: 0,
		true:  0,
	}
	for _, v := range args {
		b[v] += 1
	}
	return b[true] == 1
}

func validateFlags(closeFlag bool) error {
	// only option at the moment is `close`
	if !onlyOne(closeFlag) {
		return errors.New("update-prs needs one and only one action flag")
	}
	return nil
}

// we keep the args as one of the subfunctions might need it one day.
func run(c *cobra.Command, args []string) {
	logger := logging.NewLogger(c)
	if err := validateFlags(closeFlag); err != nil {
		logger.Errorf("Error while parsing the flags: %v", err)
		return
	}

	if closeFlag {
		runClose(c, args)
	}
}

func runClose(c *cobra.Command, _ []string) {
	logger := logging.NewLogger(c)

	readCampaignActivity := logger.StartActivity("Reading campaign data")
	dir, err := campaign.OpenCampaign()
	if err != nil {
		readCampaignActivity.EndWithFailure(err)
		return
	}
	readCampaignActivity.EndWithSuccess()

	// Prompting for confirmation
	if !yesFlag {
		// TODO: add the number of PRs that it will actually close
		if !p.AskConfirm(fmt.Sprintf("Close all PRs from the %s campaign?", dir.Name)) {
			return
		}
	}

	doneCount := 0
	skippedCount := 0
	errorCount := 0

	for _, repo := range dir.Repos {

		closeActivity := logger.StartActivity("Closing PR in %s", repo.FullRepoName)
		// skip if the working copy does not exist
		if _, err = os.Stat(repo.FullRepoPath()); os.IsNotExist(err) {
			closeActivity.EndWithWarningf("Directory %s does not exist - has it been cloned?", repo.FullRepoPath())
			skippedCount++
			continue
		}

		err = gh.ClosePullRequest(closeActivity.Writer(), repo.FullRepoPath(), dir.Name)
		if err != nil {
			if _, ok := err.(*github.NoPRFoundError); ok {
				closeActivity.EndWithWarning(err)
				skippedCount++
			} else {
				closeActivity.EndWithFailure(err)
				errorCount++
			}
		} else {
			closeActivity.EndWithSuccess()
			doneCount++
		}
	}

	if errorCount == 0 {
		logger.Successf("turbolift update-prs completed %s(%s, %s)\n", colors.Normal(), colors.Green(doneCount, " OK"), colors.Yellow(skippedCount, " skipped"))
	} else {
		logger.Warnf("turbolift update-prs completed with %s %s(%s, %s, %s)\n", colors.Red("errors"), colors.Normal(), colors.Green(doneCount, " OK"), colors.Yellow(skippedCount, " skipped"), colors.Red(errorCount, " errored"))
	}
}
