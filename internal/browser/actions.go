package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"mellowtel-consumer/internal/job"
)

// buildActionTasks translates job actions into chromedp actions, executed in
// order. Individual action failures are logged by the caller but do not abort
// the remaining steps (each action swallows its own "element missing" errors so
// a flaky selector doesn't sink an otherwise-good scrape).
func buildActionTasks(actions []job.Action) []chromedp.Action {
	tasks := make([]chromedp.Action, 0, len(actions))
	for _, a := range actions {
		tasks = append(tasks, actionTask(a))
	}
	return tasks
}

func actionTask(a job.Action) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		switch a.Type {
		case "wait":
			ms := a.Milliseconds
			if ms <= 0 {
				ms = 1000
			}
			return chromedp.Sleep(time.Duration(ms) * time.Millisecond).Do(ctx)

		case "click":
			if a.Selector == "" {
				return nil
			}
			return softClick(ctx, a.Selector)

		case "write":
			if a.Text == "" {
				return nil
			}
			return chromedp.KeyEvent(a.Text).Do(ctx)

		case "press":
			if a.Key == "" {
				return nil
			}
			return chromedp.KeyEvent(a.Key).Do(ctx)

		case "fill_input", "fill_textarea", "select", "real_input_g":
			if a.Selector == "" {
				return nil
			}
			return softSendKeys(ctx, a.Selector, a.Value)

		case "fill_form":
			for _, f := range a.Fields {
				if f.Name == "" {
					continue
				}
				if err := softSendKeys(ctx, selectorForField(a.Selector, f.Name), f.Value); err != nil {
					return err
				}
			}
			return nil

		case "scroll":
			return scroll(ctx, a.Direction, a.Amount)

		case "waitFor":
			return waitForSelector(a.Selector, a.Timeout).Do(ctx)

		default:
			// Unknown action types are ignored rather than failing the job.
			return nil
		}
	})
}

// softClick clicks a selector but treats "not found" as non-fatal.
func softClick(ctx context.Context, selector string) error {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := chromedp.Click(selector, chromedp.ByQuery, chromedp.NodeVisible).Do(cctx); err != nil {
		return nil // element missing / not clickable: skip
	}
	return nil
}

// softSendKeys sets a value on an input, treating a missing element as non-fatal.
func softSendKeys(ctx context.Context, selector, value string) error {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := chromedp.SendKeys(selector, value, chromedp.ByQuery).Do(cctx); err != nil {
		return nil
	}
	return nil
}

// scroll scrolls the window in a direction by amount pixels (default 500).
func scroll(ctx context.Context, direction string, amount int) error {
	if amount <= 0 {
		amount = 500
	}
	var x, y int
	switch direction {
	case "up":
		y = -amount
	case "left":
		x = -amount
	case "right":
		x = amount
	default: // down
		y = amount
	}
	js := fmt.Sprintf("window.scrollBy(%d, %d)", x, y)
	return chromedp.Evaluate(js, nil).Do(ctx)
}

func selectorForField(base, name string) string {
	if base != "" {
		return fmt.Sprintf(`%s [name="%s"]`, base, name)
	}
	return fmt.Sprintf(`[name="%s"]`, name)
}
