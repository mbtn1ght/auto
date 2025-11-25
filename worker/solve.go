package worker

import (
	"context"
	"errors"
	"github.com/chromedp/chromedp"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func ParseSolvedTasks(s string) []int {
	re := regexp.MustCompile(`\d+`)
	found := re.FindAllString(s, -1)
	set := make(map[int]struct{}, len(found))
	for _, f := range found {
		if n, err := strconv.Atoi(f); err == nil && n > 0 {
			set[n] = struct{}{}
		}
	}
	var res []int
	for k := range set {
		res = append(res, k)
	}
	sort.Ints(res)
	return res
}

func FetchSolvedText(allocCtx context.Context, userURL, solvedSelector string) (string, error) {
	attemptTimeouts := []time.Duration{25 * time.Second, 35 * time.Second}
	for attempt, tmo := range attemptTimeouts {
		localCtx, cancelCtx := chromedp.NewContext(allocCtx)
		runCtx, cancelRun := context.WithTimeout(localCtx, tmo)

		var solvedText string
		err := chromedp.Run(runCtx,
			chromedp.Navigate(userURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Text(solvedSelector, &solvedText, chromedp.NodeVisible, chromedp.ByQuery),
		)

		cancelRun()
		cancelCtx()

		if err == nil && strings.TrimSpace(solvedText) != "" {
			log.Printf("chromedp selector attempt %d succeeded, raw chars=%d", attempt+1, len(solvedText))
			trim := solvedText
			if len(trim) > 800 {
				log.Printf("raw solvedText (start): %q", strings.TrimSpace(trim[:400]))
				log.Printf("raw solvedText (end): %q", strings.TrimSpace(trim[len(trim)-400:]))
			} else {
				log.Printf("raw solvedText: %q", strings.TrimSpace(trim))
			}
			return solvedText, nil
		}
		log.Printf("chromedp selector attempt %d failed: %v", attempt+1, err)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	jsExpr := `(function(){
		var candidates = Array.from(document.querySelectorAll('p,td,div,li'));
		var el = candidates.find(e => /реш/i.test(e.textContent));
		var nums = [];
		if (el) {
			var m = el.textContent.match(/\d+/g);
			if (m) { nums = nums.concat(m); }
		}
		if (nums.length === 0) {
			var anchors = Array.from(document.querySelectorAll('a'));
			for (var i=0;i<anchors.length;i++){
				var t = anchors[i].textContent.trim();
				if (/^\d+$/.test(t)) nums.push(t);
			}
		}
		return nums.join(' ');
	})()`

	for attempt, tmo := range []time.Duration{30 * time.Second, 40 * time.Second} {
		localCtx, cancelCtx := chromedp.NewContext(allocCtx)
		runCtx, cancelRun := context.WithTimeout(localCtx, tmo)

		var solvedText string
		err := chromedp.Run(runCtx,
			chromedp.Navigate(userURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Evaluate(jsExpr, &solvedText),
		)

		cancelRun()
		cancelCtx()

		if err == nil && strings.TrimSpace(solvedText) != "" {
			log.Printf("chromedp js-scan attempt %d succeeded, found %d chars", attempt+1, len(solvedText))
			return solvedText, nil
		}
		log.Printf("chromedp js-scan attempt %d failed: %v", attempt+1, err)
		time.Sleep(time.Second * time.Duration(attempt+1))
	}

	httpClient := &http.Client{
		Timeout: 20 * time.Second,
	}
	resp, err := httpClient.Get(userURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", errors.New("http fallback returned status " + resp.Status)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	body := string(bodyBytes)

	body = strings.ReplaceAll(body, "\u00A0", " ")

	reSection := regexp.MustCompile(`(?is)(Решен|Решён|Решенные|Решённые|решен|решё)(.*?)</(p|td|div|li)>`)
	if m := reSection.FindStringSubmatch(body); len(m) >= 3 {
		section := m[2]
		reNums := regexp.MustCompile(`\d+`)
		found := reNums.FindAllString(section, -1)
		if len(found) > 0 {
			return strings.Join(found, " "), nil
		}
	}

	reAnchors := regexp.MustCompile(`<a[^>]*>\s*([0-9]{1,6})\s*</a>`)
	all := reAnchors.FindAllStringSubmatch(body, -1)
	if len(all) > 0 {
		var parts []string
		for _, sub := range all {
			if len(sub) > 1 {
				parts = append(parts, sub[1])
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " "), nil
		}
	}

	reAll := regexp.MustCompile(`[0-9]+`)
	allNums := reAll.FindAllString(body, -1)
	if len(allNums) == 0 {
		return "", errors.New("не удалось найти список решённых задач на странице")
	}
	return strings.Join(allNums, " "), nil
}
