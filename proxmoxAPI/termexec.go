package proxmoxAPI

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/luthermonson/go-proxmox"
)

const (
	kothBeginMarker = "__KOTH_BEGIN__"
	kothSplitMarker = "__KOTH_SPLIT__"
	kothDoneMarker  = "__KOTH_DONE__"
	kothRCMarker    = "__KOTH_RC="
)

type termProxySession struct {
	Port   proxmox.StringOrInt `json:"port"`
	Ticket string              `json:"ticket"`
	User   string              `json:"user"`
}

func (api *ProxmoxAPI) RawExecute(ct *proxmox.Container, username, password, command string) (stdout string, stderr string, exitCode int, err error) {
	var (
		nodeName string
		host     string
		vmid     int
	)

	if nodeName, host, vmid, err = api.resolveNodeAndHost(ct); err != nil {
		return
	}

	baseCtx := api.bg
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	connectCtx, cancelConnect := context.WithTimeout(baseCtx, api.getConnectTimeout())
	defer cancelConnect()

	var (
		authTicket string
		csrfToken  string
	)
	if authTicket, csrfToken, err = api.loginTicket(connectCtx, host); err != nil {
		err = fmt.Errorf("failed to obtain PVE ticket: %w", err)
		return
	}

	var session *termProxySession
	if session, err = api.openTermProxy(connectCtx, host, nodeName, vmid, authTicket, csrfToken); err != nil {
		err = fmt.Errorf("failed to open termproxy: %w", err)
		return
	}

	wsCtx, cancelWS := context.WithTimeout(baseCtx, api.getConnectTimeout())
	defer cancelWS()

	var conn *websocket.Conn
	if conn, err = api.dialVNCWebSocket(wsCtx, host, nodeName, vmid, session, authTicket); err != nil {
		err = fmt.Errorf("failed to dial console websocket: %w", err)
		return
	}
	defer conn.Close()

	terminalUser := determineTerminalUser(session.User, api.username, api.tokenUser)
	if terminalUser == "" {
		err = fmt.Errorf("failed to determine terminal user for handshake")
		return
	}

	cmdCtx, cancelCmd := context.WithTimeout(baseCtx, api.getCommandTimeout())
	defer cancelCmd()

	stdout, stderr, exitCode, err = api.runInteractiveCommand(cmdCtx, conn, terminalUser, session.Ticket, username, password, command)
	return
}

func (api *ProxmoxAPI) resolveNodeAndHost(ct *proxmox.Container) (nodeName string, host string, vmid int, err error) {
	if ct == nil {
		err = fmt.Errorf("container is nil")
		return
	}

	vmid = int(ct.VMID)
	if vmid <= 0 {
		err = fmt.Errorf("container VMID is invalid")
		return
	}

	nodeName = strings.TrimSpace(ct.Node)
	if nodeName == "" {
		var node *proxmox.Node
		if node, err = api.NodeForContainer(vmid); err == nil && node != nil {
			nodeName = node.Name
		}
	}

	if nodeName == "" {
		err = fmt.Errorf("could not determine node for container %d", vmid)
		return
	}

	host = nodeName
	if api.apiHost != "" && strings.EqualFold(api.apiHost, nodeName) {
		host = api.apiHost
	}

	return
}

func (api *ProxmoxAPI) loginTicket(ctx context.Context, host string) (ticket, csrf string, err error) {
	if api.username == "" || api.password == "" {
		err = fmt.Errorf("missing proxmox username/password for ticket login")
		return
	}

	form := url.Values{}
	form.Set("username", api.username)
	form.Set("password", api.password)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s:%s/api2/json/access/ticket", host, api.port()), strings.NewReader(form.Encode()))
	if reqErr != nil {
		err = reqErr
		return
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp *http.Response
	if resp, err = api.restClient().Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body []byte
		body, _ = io.ReadAll(resp.Body)
		err = fmt.Errorf("ticket request failed (%s): %s", resp.Status, string(body))
		return
	}

	var parsed struct {
		Data struct {
			Ticket string `json:"ticket"`
			CSRF   string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return
	}

	ticket = parsed.Data.Ticket
	csrf = parsed.Data.CSRF

	if ticket == "" || csrf == "" {
		err = fmt.Errorf("received empty ticket or CSRF token from Proxmox")
	}

	return
}

func (api *ProxmoxAPI) openTermProxy(ctx context.Context, host, node string, vmid int, ticket, csrf string) (session *termProxySession, err error) {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s:%s/api2/json/nodes/%s/lxc/%d/termproxy", host, api.port(), node, vmid), nil)
	if reqErr != nil {
		err = reqErr
		return
	}

	req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", ticket))
	req.Header.Set("CSRFPreventionToken", csrf)

	var resp *http.Response
	if resp, err = api.restClient().Do(req); err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body []byte
		body, _ = io.ReadAll(resp.Body)
		err = fmt.Errorf("termproxy request failed (%s): %s", resp.Status, string(body))
		return
	}

	var parsed struct {
		Data termProxySession `json:"data"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return
	}

	if parsed.Data.Ticket == "" || parsed.Data.Port == 0 {
		err = fmt.Errorf("termproxy response missing ticket or port")
		return
	}

	session = &parsed.Data
	return
}

func (api *ProxmoxAPI) dialVNCWebSocket(ctx context.Context, host, node string, vmid int, session *termProxySession, authTicket string) (conn *websocket.Conn, err error) {
	if session == nil {
		err = fmt.Errorf("termproxy session is nil")
		return
	}

	u := fmt.Sprintf("wss://%s:%s/api2/json/nodes/%s/lxc/%d/vncwebsocket?port=%d&vncticket=%s",
		host, api.port(), node, vmid, int(session.Port), url.QueryEscape(session.Ticket))

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: api.getConnectTimeout(),
		TLSClientConfig:  api.tlsConfig(),
	}

	headers := http.Header{}
	headers.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", authTicket))
	headers.Set("Origin", fmt.Sprintf("https://%s:%s", host, api.port()))
	headers.Set("User-Agent", "pve-koth/termexec")

	conn, _, err = dialer.DialContext(ctx, u, headers)
	return
}

func (api *ProxmoxAPI) runInteractiveCommand(ctx context.Context, conn *websocket.Conn, termUser, termTicket, containerUser, containerPass, command string) (stdout, stderr string, exitCode int, err error) {
	if conn == nil {
		err = fmt.Errorf("websocket connection is nil")
		return
	}

	if err = api.performHandshake(ctx, conn, termUser, termTicket); err != nil {
		return
	}

	keepAliveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go api.keepAlive(keepAliveCtx, conn)

	if err = api.ensureConsoleLogin(ctx, conn, containerUser, containerPass); err != nil {
		return
	}

	stdout, stderr, exitCode, err = api.executeAndCollect(ctx, conn, command)
	return
}

func (api *ProxmoxAPI) performHandshake(ctx context.Context, conn *websocket.Conn, termUser, termTicket string) (err error) {
	if termUser == "" || termTicket == "" {
		err = fmt.Errorf("missing terminal user or ticket for handshake")
		return
	}

	if err = api.sendFrame(ctx, conn, []byte(fmt.Sprintf("%s:%s\n", termUser, termTicket))); err != nil {
		return
	}

	var msg []byte
	if msg, err = api.readMessage(ctx, conn); err != nil {
		return
	}

	if strings.TrimSpace(string(msg)) != "OK" {
		err = fmt.Errorf("terminal handshake not acknowledged: %s", strings.TrimSpace(string(msg)))
		return
	}

	// Set a sensible default terminal size; the console protocol expects a resize frame early.
	if err = api.sendFrame(ctx, conn, []byte("1:32:120:")); err != nil {
		return
	}

	return
}

func (api *ProxmoxAPI) ensureConsoleLogin(ctx context.Context, conn *websocket.Conn, user, pass string) (err error) {
	if user == "" {
		err = fmt.Errorf("container login user is required")
		return
	}

	// Nudge the console to show a prompt.
	_ = api.sendInput(ctx, conn, "\n")

	var buffer strings.Builder

	for {
		var msg []byte
		if msg, err = api.readMessage(ctx, conn); err != nil {
			return
		}

		text := stripANSI(string(msg))
		buffer.WriteString(text)

		lower := strings.ToLower(buffer.String())
		if strings.Contains(lower, "login:") {
			buffer.Reset()
			if err = api.sendInput(ctx, conn, fmt.Sprintf("%s\n", user)); err != nil {
				return
			}
			continue
		}

		if strings.Contains(lower, "password:") {
			buffer.Reset()
			if err = api.sendInput(ctx, conn, fmt.Sprintf("%s\n", pass)); err != nil {
				return
			}
			continue
		}

		if strings.Contains(lower, "login incorrect") || strings.Contains(lower, "authentication failure") {
			err = fmt.Errorf("failed to authenticate to container console as %s", user)
			return
		}

		if hasShellPrompt(buffer.String()) {
			return
		}

		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
	}
}

func (api *ProxmoxAPI) executeAndCollect(ctx context.Context, conn *websocket.Conn, command string) (stdout, stderr string, exitCode int, err error) {
	wrapped := buildWrappedCommand(command)
	if err = api.sendInput(ctx, conn, wrapped); err != nil {
		return
	}

	var output strings.Builder

	for {
		var msg []byte
		if msg, err = api.readMessage(ctx, conn); err != nil {
			return
		}

		output.Write(msg)

		if strings.Contains(output.String(), kothDoneMarker) {
			break
		}

		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
	}

	stdout, stderr, exitCode, err = parseCommandOutput(output.String())
	return
}

func (api *ProxmoxAPI) keepAlive(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = api.sendFrame(ctx, conn, []byte("2"))
		}
	}
}

func (api *ProxmoxAPI) sendInput(ctx context.Context, conn *websocket.Conn, data string) error {
	payload := append([]byte(fmt.Sprintf("0:%d:", len(data))), []byte(data)...)
	return api.sendFrame(ctx, conn, payload)
}

func (api *ProxmoxAPI) sendFrame(ctx context.Context, conn *websocket.Conn, payload []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && time.Until(d) < 5*time.Second {
		deadline = d
	}

	_ = conn.SetWriteDeadline(deadline)
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (api *ProxmoxAPI) readMessage(ctx context.Context, conn *websocket.Conn) (msg []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("websocket read panic: %v", r)
			_ = conn.Close()
		}
	}()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	deadline := time.Now().Add(api.getCommandTimeout())
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}

	cancelRead := make(chan struct{})
	defer close(cancelRead)

	// If the context is canceled before the read returns, force the read to unblock.
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Now())
		case <-cancelRead:
		}
	}()

	_ = conn.SetReadDeadline(deadline)

	var mType int
	if mType, msg, err = conn.ReadMessage(); err != nil {
		_ = conn.Close()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return
	}

	if mType == websocket.CloseMessage {
		err = fmt.Errorf("websocket closed")
		_ = conn.Close()
		return
	}

	return
}

func buildWrappedCommand(command string) string {
	cmdB64 := base64.StdEncoding.EncodeToString([]byte(command))

	innerScript := fmt.Sprintf("#!/bin/sh\ncmd_b64='%s'\ncmd=$(echo \"$cmd_b64\" | base64 -d)\nprintf '%s\\n'\n{ eval \"$cmd\"; } 1>/tmp/koth_out 2>/tmp/koth_err\nrc=$?\nprintf '%s%%s__\\n' \"$rc\"\ncat /tmp/koth_out\nprintf '%s\\n'\ncat /tmp/koth_err\nprintf '%s\\n'\nrm -f /tmp/koth_out /tmp/koth_err\n", cmdB64, kothBeginMarker, kothRCMarker, kothSplitMarker, kothDoneMarker)
	wrapperB64 := base64.StdEncoding.EncodeToString([]byte(innerScript))

	return fmt.Sprintf("wrap_b64='%s'; printf '%%s' \"$wrap_b64\" | base64 -d | sh\n", wrapperB64)
}

func parseCommandOutput(raw string) (stdout, stderr string, exitCode int, err error) {
	cleaned := stripANSI(raw)
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	beginIdx := strings.LastIndex(cleaned, kothBeginMarker)
	doneIdx := strings.LastIndex(cleaned, kothDoneMarker)

	// Fallback: if we never saw markers, return everything as stdout with rc 0.
	if beginIdx == -1 && doneIdx == -1 {
		stdout = strings.TrimSpace(cleaned)
		stderr = ""
		exitCode = 0
		return
	}

	if beginIdx == -1 && doneIdx != -1 {
		beginIdx = 0
	}

	if doneIdx == -1 || doneIdx <= beginIdx {
		err = fmt.Errorf("failed to locate command markers in console output")
		return
	}

	segment := cleaned[beginIdx+len(kothBeginMarker) : doneIdx]

	exitCode = -1

	if rcIdx := strings.Index(segment, kothRCMarker); rcIdx >= 0 {
		rcSection := segment[rcIdx+len(kothRCMarker):]
		if nl := strings.Index(rcSection, "__"); nl >= 0 {
			rcText := strings.TrimSpace(rcSection[:nl])
			if parsed, convErr := strconv.Atoi(rcText); convErr == nil {
				exitCode = parsed
			}
			rcSection = rcSection[nl+2:]
		}
		if nl := strings.Index(rcSection, "\n"); nl >= 0 {
			segment = rcSection[nl+1:]
		} else {
			segment = rcSection
		}
	}

	splitIdx := strings.Index(segment, kothSplitMarker)
	if splitIdx == -1 {
		// Fallback: if we got an exit code but no split marker, treat the whole payload as stdout.
		stdout = strings.TrimPrefix(segment, "\n")
		stderr = ""
		if exitCode == -1 {
			exitCode = 0
		}
		return
	}

	stdout = segment[:splitIdx]
	stderr = segment[splitIdx+len(kothSplitMarker):]

	stdout = strings.TrimPrefix(stdout, "\n")
	stderr = strings.TrimPrefix(stderr, "\n")

	if exitCode == -1 {
		exitCode = 0
	}

	return
}

func stripANSI(s string) string {
	var (
		ansiPattern   = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
		controlChars  = regexp.MustCompile(`[\x00-\x09\x0b-\x1f\x7f]`)
		resetPattern  = regexp.MustCompile(`\x1b\]0;.*\x07`)
		osCommandRepl = regexp.MustCompile(`\x1b\[[>=][0-9;]*`)
	)

	s = ansiPattern.ReplaceAllString(s, "")
	s = resetPattern.ReplaceAllString(s, "")
	s = osCommandRepl.ReplaceAllString(s, "")
	s = controlChars.ReplaceAllString(s, "")

	return s
}

func hasShellPrompt(s string) bool {
	clean := stripANSI(s)
	lines := strings.Split(clean, "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		if strings.HasSuffix(line, "#") || strings.HasSuffix(line, "$") {
			return true
		}
	}

	return false
}

func (api *ProxmoxAPI) restClient() *http.Client {
	if api.httpClient != nil {
		if api.httpClient.Timeout == 0 {
			api.httpClient.Timeout = api.getConnectTimeout()
		}
		return api.httpClient
	}

	api.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: api.tlsConfig(),
		},
		Timeout: api.getConnectTimeout(),
	}

	return api.httpClient
}

func (api *ProxmoxAPI) tlsConfig() *tls.Config {
	if api.httpClient != nil {
		if transport, ok := api.httpClient.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil {
			return transport.TLSClientConfig
		}
	}

	return &tls.Config{
		InsecureSkipVerify: api.InsecureSkipVerify,
	}
}

func (api *ProxmoxAPI) port() string {
	if api.apiPort != "" {
		return api.apiPort
	}

	return "8006"
}

func (api *ProxmoxAPI) getConnectTimeout() time.Duration {
	if api.ConnectTimeout > 0 {
		return api.ConnectTimeout
	}

	return 30 * time.Second
}

func (api *ProxmoxAPI) getCommandTimeout() time.Duration {
	if api.CommandTimeout > 0 {
		return api.CommandTimeout
	}

	return time.Minute
}
