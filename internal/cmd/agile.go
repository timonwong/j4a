package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/jira"
	"github.com/timonwong/jiro/internal/output"
)

var numericBoardSelector = regexp.MustCompile(`^[+-]?[0-9]+$`)

type boardSelection struct {
	explicit bool
	value    string
	id       int
}

type boardListResult struct {
	Total  int          `json:"total"`
	Boards []jira.Board `json:"boards"`
}

type sprintListResult struct {
	Total        int                  `json:"total"`
	Sprints      []jira.Sprint        `json:"sprints"`
	FailedBoards []sprintBoardFailure `json:"failedBoards,omitempty"`
}

type sprintBoardFailure struct {
	BoardID   int    `json:"boardId"`
	BoardName string `json:"boardName"`
	Error     string `json:"error"`
}

func (a *app) boardCommand() *cobra.Command {
	command := &cobra.Command{Use: "board", Short: "Inspect Jira Software Boards"}
	command.AddCommand(a.boardListCommand())
	return command
}

func (a *app) boardListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Jira Software Boards",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			boards, err := client.ListAllBoards(command.Context())
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(boards))
			for _, board := range boards {
				rows = append(rows, []string{strconv.Itoa(board.ID), board.Name, board.Type})
			}
			return a.renderDiscovery(boardListResult{Total: len(boards), Boards: boards}, output.Table{
				Columns: []output.Column{output.Fixed("ID"), output.Flexible("NAME"), output.Flexible("TYPE")},
				Rows:    rows,
			}, false)
		},
	}
}

func (a *app) sprintCommand() *cobra.Command {
	command := &cobra.Command{Use: "sprint", Short: "Inspect Jira Software Sprints"}
	command.AddCommand(a.sprintListCommand())
	return command
}

func (a *app) sprintListCommand() *cobra.Command {
	var boardSelector, stateValue string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Jira Software Sprints",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			state, err := jira.ParseSprintState(stateValue)
			if err != nil {
				return err
			}
			selection, err := parseBoardSelection(boardSelector, command.Flags().Changed("board"))
			if err != nil {
				return err
			}
			client, _, err := a.client()
			if err != nil {
				return err
			}
			boards, err := client.ListAllBoards(command.Context())
			if err != nil {
				return err
			}
			boards, err = selection.selectBoards(boards)
			if err != nil {
				return err
			}
			sprints := make([]jira.Sprint, 0)
			failures := make([]sprintBoardFailure, 0)
			successfulBoards := 0
			var firstErr error
			for _, board := range boards {
				listed, err := client.ListAllSprints(command.Context(), board.ID, state)
				for i := range listed {
					listed[i].BoardName = board.Name
				}
				sprints = append(sprints, listed...)
				if err != nil {
					if command.Context().Err() != nil {
						return command.Context().Err()
					}
					if firstErr == nil {
						firstErr = err
					}
					failures = append(failures, sprintBoardFailure{BoardID: board.ID, BoardName: board.Name, Error: err.Error()})
					if len(listed) > 0 {
						successfulBoards++
					}
					continue
				}
				successfulBoards++
			}
			result := sprintListResult{Total: len(sprints), Sprints: sprints, FailedBoards: failures}
			if len(failures) == 0 {
				return a.renderDiscovery(result, sprintTable(sprints), false)
			}
			if successfulBoards == 0 {
				return firstErr
			}
			if err := a.renderDiscovery(result, sprintTable(sprints), true); err != nil {
				return err
			}
			return apperr.New(apperr.KindPartialFailure, fmt.Sprintf("sprint list completed with %d failed Board(s)", len(failures)))
		},
	}
	command.Flags().StringVar(&boardSelector, "board", "", "Board ID or case-insensitive name substring")
	command.Flags().StringVar(&stateValue, "state", string(jira.SprintStateActive), "Sprint state: active, closed, future, or all")
	return command
}

func parseBoardSelection(selector string, explicit bool) (boardSelection, error) {
	if !explicit {
		return boardSelection{}, nil
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return boardSelection{}, apperr.New(apperr.KindInvalidInput, "board selector must not be empty")
	}
	if !numericBoardSelector.MatchString(selector) {
		return boardSelection{explicit: true, value: selector}, nil
	}
	id, err := strconv.ParseInt(selector, 10, 0)
	if err != nil || id <= 0 {
		return boardSelection{}, apperr.New(apperr.KindInvalidInput, "board ID must be positive")
	}
	return boardSelection{explicit: true, value: selector, id: int(id)}, nil
}

func (s boardSelection) selectBoards(boards []jira.Board) ([]jira.Board, error) {
	if !s.explicit {
		return boards, nil
	}
	if s.id > 0 {
		for _, board := range boards {
			if board.ID == s.id {
				return []jira.Board{board}, nil
			}
		}
		return nil, apperr.New(apperr.KindNotFound, fmt.Sprintf("board %q was not found", s.value))
	}
	needle := strings.ToLower(s.value)
	selected := make([]jira.Board, 0)
	for _, board := range boards {
		if strings.Contains(strings.ToLower(board.Name), needle) {
			selected = append(selected, board)
		}
	}
	if len(selected) == 0 {
		return nil, apperr.New(apperr.KindNotFound, fmt.Sprintf("board %q was not found", s.value))
	}
	return selected, nil
}

func sprintTable(sprints []jira.Sprint) output.Table {
	rows := make([][]string, 0, len(sprints))
	for _, sprint := range sprints {
		rows = append(rows, []string{
			strconv.Itoa(sprint.ID), sprint.Name, sprint.State, strconv.Itoa(sprint.BoardID), sprint.BoardName,
			sprint.StartDate, sprint.EndDate, sprint.CompleteDate,
		})
	}
	return output.Table{
		Columns: []output.Column{
			output.Fixed("ID"), output.Flexible("NAME"), output.Fixed("STATE"), output.Fixed("BOARD ID"),
			output.Flexible("BOARD NAME"), output.Flexible("START"), output.Flexible("END"), output.Flexible("COMPLETE"),
		},
		Rows: rows,
	}
}

func (a *app) renderDiscovery(data any, table output.Table, partial bool) error {
	renderer, err := a.renderer()
	if err != nil {
		return err
	}
	if partial {
		renderer.Quiet = false
	}
	if renderer.Format == output.FormatText {
		if len(table.Rows) == 0 {
			return nil
		}
		return renderer.Success(table)
	}
	return renderer.Success(data)
}
