//go:build e2e

package framework

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type HTTPGetOptions struct {
	Timeout      time.Duration
	Headers      map[string]string
	Insecure     bool
	Method       string
	Body         string
	FollowRedirects bool
}

type HTTPResponse struct {
	StatusCode int
	Body       string
	Headers    map[string]string
}

func HTTPGetFromPod(t T, podNS, podName, url string, opts ...func(*HTTPGetOptions)) *HTTPResponse {
	t.Helper()

	o := &HTTPGetOptions{
		Timeout: 10 * time.Second,
		Method:  "GET",
		FollowRedirects: true,
	}
	for _, fn := range opts {
		fn(o)
	}

	curlCmd := fmt.Sprintf("curl -sS --connect-timeout %d --max-time %d -o /tmp/response-body.txt -w '%%{http_code}'",
		int(o.Timeout.Seconds()), int(o.Timeout.Seconds()))

	if o.Insecure {
		curlCmd += " -k"
	}
	if o.Method != "GET" {
		curlCmd += fmt.Sprintf(" -X %s", o.Method)
	}
	if !o.FollowRedirects {
		curlCmd += " --max-redirs 0"
	}
	for k, v := range o.Headers {
		curlCmd += fmt.Sprintf(" -H '%s: %s'", k, v)
	}
	if o.Body != "" {
		curlCmd += fmt.Sprintf(" -d '%s'", o.Body)
	}

	curlCmd += fmt.Sprintf(" '%s'", url)

	fullCmd := fmt.Sprintf("rm -f /tmp/response-body.txt && %s && cat /tmp/response-body.txt", curlCmd)

	cmd := exec.Command("kubectl", "exec", "-n", podNS, podName, "--", "sh", "-c", fullCmd)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		t.Logf("curl from pod %s/%s to %s failed: %v\noutput: %s", podNS, podName, url, err, outputStr)
		return &HTTPResponse{StatusCode: 0, Body: outputStr}
	}

	lines := strings.SplitN(strings.TrimSpace(outputStr), "\n", 2)
	statusLine := strings.TrimSpace(lines[0])

	var body string
	if len(lines) > 1 {
		body = lines[1]
	}

	statusCode := 0
	if _, err := fmt.Sscanf(statusLine, "%d", &statusCode); err == nil {
	} else {
		t.Logf("could not parse HTTP status code from: %q", statusLine)
		return &HTTPResponse{StatusCode: 0, Body: outputStr}
	}

	return &HTTPResponse{
		StatusCode: statusCode,
		Body:       body,
		Headers:    make(map[string]string),
	}
}

func ProbeUntil(t T, podNS, podName, url string, expectedStatus int, opts ...func(*HTTPGetOptions)) {
	t.Helper()

	deadline := time.Now().Add(120 * time.Second)
	interval := 2 * time.Second

	for time.Now().Before(deadline) {
		resp := HTTPGetFromPod(t, podNS, podName, url, opts...)
		if resp.StatusCode == expectedStatus {
			t.Logf("probe %s returned expected status %d", url, expectedStatus)
			return
		}
		t.Logf("probe %s returned status %d, waiting for %d", url, resp.StatusCode, expectedStatus)
		time.Sleep(interval)
		interval = minDuration(interval*2, 15*time.Second)
	}

	t.Fatalf("probe %s did not return status %d within timeout", url, expectedStatus)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
