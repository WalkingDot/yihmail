package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

//go:embed ca-certificates.crt
var cacertificates []byte

// command line arguments
var (
	mailfrom, mailto, mailusr, mailpw, mailhost, mailport, mailres, mailip, runmode, uselog, rtspport, eventskip, oneshotevent, readqueue string
	waitdur, timezfix                                                                                                                     time.Duration
)

// flag pkg recommends init declaration
func init() {
	flag.StringVar(&mailfrom, "f", "", "Email from.\n\texample: YourAccount@domain.com")
	flag.StringVar(&mailto, "t", "", "Email to.\n\texample: Account@domain.com")
	flag.StringVar(&mailusr, "u", "", "Account name.\n\texample: YourAccount@domain.com or only YourAccount")
	flag.StringVar(&mailpw, "p", "", "Account password.")
	flag.StringVar(&mailhost, "h", "", "SMTP host.\n\texample: smtp.domain.com or smtp-mail.domain.com")
	flag.StringVar(&mailport, "n", "587", "SMTP port.\n\t(optional - default: 587)")
	flag.StringVar(&runmode, "m", "daemon", "Run permanently in background or send only one email.\n\tIf started as oneshot, exit status is 0 on success.\n\toptions: daemon, oneshot (optional - default: daemon)")
	flag.StringVar(&mailres, "r", "low", "Set the preferred resolution.\n\toptions: low, high, none (optional - default: low)")
	flag.StringVar(&mailip, "i", "off", "Get a RTSP link with public IP.\n\toptions: on, off (optional - default: off)")
	flag.DurationVar(&waitdur, "w", time.Duration(600*time.Second), "Waits in seconds between emails.\n\toptions: 0s - 99999s (optional - default: 600s)")
	flag.StringVar(&eventskip, "s", "", "To skip events choose initial letters (Motion, Sound, Human, Baby).\n\toptions: mshb (optional)\n\texample for only motion events: shb")
	flag.StringVar(&rtspport, "v", "554", "RTSP port.\n\t(optional - default: 554)")
	flag.DurationVar(&timezfix, "z", time.Duration(0*time.Hour), "Time zone fix, if you have a wrong time.\n\toption hours: -23h to 23h (optional - default: 0h)")
	flag.StringVar(&oneshotevent, "e", "", "Set a custom oneshot event name.\n\t(optional - default: none)")
	flag.StringVar(&uselog, "l", "off", "Use log file; available at: http://IP-CAM:8080/log/mail.txt\n\toptions: on, off (optional - default: off)")
}

func logmsg(m string) {
	fmt.Println(m)
	if uselog == "on" {
		// txt file type for webbrowser compatibility
		lof, err := os.OpenFile("/tmp/sd/yi-hack/www/log/mail.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		if _, err := lof.WriteString(time.Now().Add(timezfix).Format("2006/01/02 15:04:05 - ") + m + "\n"); err != nil {
			lof.Close()
			fmt.Fprintln(os.Stderr, err)
			return
		}

		if err := lof.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		log, err := os.Stat("/tmp/sd/yi-hack/www/log/mail.txt")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}

		if log.Size() > 100000 {
			logs, err := os.ReadDir("/tmp/sd/yi-hack/www/log")
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}

			if err := os.Rename("/tmp/sd/yi-hack/www/log/mail.txt", "/tmp/sd/yi-hack/www/log/mailold"+strconv.Itoa(len(logs)-1)+".txt"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
		}
	}
}

func flaghelp() {
	fmt.Fprintln(flag.CommandLine.Output(), "Usage of yihmail:\nyihmail -f -t -u -p -h [-n -m -r -i -w -s -v -z -e -l]")
	fo := [15]string{"f", "t", "u", "p", "h", "n", "m", "r", "i", "w", "s", "v", "z", "e", "l"}
	for _, f := range fo {
		flg := flag.Lookup(f)
		fmt.Fprintf(flag.CommandLine.Output(), "-%s\t%s\n", flg.Name, flg.Usage)
	}
	os.Exit(1)
}

func main() {
	flag.Usage = flaghelp
	flag.Parse()

	// required command line arguments
	switch {
	case len(mailfrom) == 0:
		flaghelp()
	case len(mailto) == 0:
		flaghelp()
	case len(mailusr) == 0:
		flaghelp()
	case len(mailpw) == 0:
		flaghelp()
	case len(mailhost) == 0:
		flaghelp()
	}

	// check/create log directory
	if uselog == "on" {
		// getting symbolic link error: operation not permitted
		// workaround - create dir directly in www
		if err := os.MkdirAll("/tmp/sd/yi-hack/www/log", 0744); err != nil {
			uselog = "off"
			fmt.Fprintln(os.Stderr, "Log is disabled due to an error")
			logmsg(err.Error() + " (err 1)")
		}
	}
	logmsg("yihmail started")

	// create ca-certificates.crt to send mails.
	// TODO: place it on sd card instead of internal storage.
	if _, err := os.Stat("/etc/ssl/certs/ca-certificates.crt"); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll("/etc/ssl/certs", 0744); err != nil {
			logmsg(err.Error() + " (err 2)")
		}
		if err := os.WriteFile("/etc/ssl/certs/ca-certificates.crt", cacertificates, 0666); err != nil {
			logmsg(err.Error() + " (err 3)")
		}
	}

	getsuffix, err := os.ReadFile("/tmp/sd/yi-hack/model_suffix")
	if err != nil {
		getsuffix = []byte("y21ga")
	}
	camsuf := string(getsuffix)

	// sort user input excluded events:
	var validevent = [4]bool{true, true, true, true}
	for _, v := range eventskip {
		switch {
		case v == 109:
			validevent[0] = false
		case v == 115:
			validevent[1] = false
		case v == 104:
			validevent[2] = false
		case v == 98:
			validevent[3] = false
		}
	}

	// snapshot buffer
	var mbuf []byte
	if mailres == "low" {
		mbuf = make([]byte, 0, 65536)
	} else if mailres == "high" {
		mbuf = make([]byte, 0, 491520)
	} else {
		// typo fallback
		mailres = "none"
	}
	var rbuf = make([]byte, 570)

	// run once only as oneshot
	if runmode == "oneshot" {
		if err := eventdetected(oneshotevent, mbuf, rbuf, time.Now().Add(timezfix), camsuf); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// daemon option:  prepare ipc_multiplexer to run as child
	pids, _ := filepath.Glob("/proc/[0-9]*")
	for _, pid := range pids {
		if pexe, _ := os.Readlink(pid + "/exe"); pexe == "/tmp/sd/yi-hack/bin/ipc_multiplexer" {
			pint, _ := strconv.Atoi(filepath.Base(pid))
			prc, _ := os.FindProcess(pint)
			prc.Kill()
			for i := 0; i < 10; i++ {
				if _, err := os.Stat(pid); errors.Is(err, os.ErrNotExist) {
					break
				}
				if i == 9 {
					logmsg("Can not start ipc_multiplexer as child (err 4)")
					os.Exit(1)
				}
				time.Sleep(500 * time.Millisecond)
			}
			break
		}
	}
	cmdm := exec.Command("/tmp/sd/yi-hack/bin/ipc_multiplexer")
	sem, err := cmdm.StderrPipe()
	if err != nil {
		logmsg(err.Error() + " (err 5)")
		os.Exit(1)
	}

	if err := cmdm.Start(); err != nil {
		logmsg(err.Error() + " (err 6)")
		os.Exit(1)
	}

	var (
		eventmotion = []byte{48, 49, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 50, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 55, 99, 32, 48, 48, 32, 55, 99, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32}
		eventsound  = []byte{48, 52, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 50, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 52, 32, 54, 48, 32, 48, 52, 32, 54, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32}
		eventhuman  = []byte{48, 49, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 50, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 102, 53, 32, 48, 48, 32, 102, 53, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32}
		eventbaby   = []byte{48, 52, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 50, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 50, 32, 54, 48, 32, 48, 50, 32, 54, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32, 48, 48, 32}
		t, waiting  time.Time
	)

	scanner := bufio.NewScanner(sem)
	scanner.Buffer(make([]byte, 128), 512)
	for scanner.Scan() {
		switch t = time.Now(); {
		case waiting.After(t):
			// skip
		case bytes.Equal(scanner.Bytes(), eventmotion):
			if validevent[0] {
				waiting = t.Add(waitdur)
				logmsg("Motion detected! Sending email")
				eventdetected("Motion", mbuf, rbuf, t, camsuf)
			}
		case bytes.Equal(scanner.Bytes(), eventsound):
			if validevent[1] {
				waiting = t.Add(waitdur)
				logmsg("Sound detected! Sending email")
				eventdetected("Sound", mbuf, rbuf, t, camsuf)
			}
		case bytes.Equal(scanner.Bytes(), eventhuman):
			if validevent[2] {
				waiting = t.Add(waitdur)
				logmsg("Human detected! Sending email")
				eventdetected("Human", mbuf, rbuf, t, camsuf)
			}
		case bytes.Equal(scanner.Bytes(), eventbaby):
			if validevent[3] {
				waiting = t.Add(waitdur)
				logmsg("Baby cry detected<! Sending email")
				eventdetected("Baby", mbuf, rbuf, t, camsuf)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logmsg(err.Error() + " (err 7)")
		os.Exit(1)
	}

	if err := cmdm.Wait(); err != nil {
		logmsg(err.Error() + " (err 8)")
		os.Exit(1)
	}

	logmsg("Unexpected Exit (err 99)")
	os.Exit(1)
}

func eventdetected(eventtype string, mbuf []byte, rbuf []byte, t time.Time, camsuf string) error {
	t = t.Add(timezfix)
	var boundary string
	camname, _ := os.Hostname()
	encabc := [64]byte{65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 43, 47}

	// build mail msg
	mbuf = append(mbuf, []byte("MIME-Version: 1.0\r\n")...)
	mbuf = append(mbuf, []byte("Date: "+t.Format(time.RFC1123Z)+"\r\n")...)
	mbuf = append(mbuf, []byte("Subject: Alert: Event Detected\r\n")...)
	mbuf = append(mbuf, []byte("From: Cam - "+camname+" <"+mailfrom+">\r\n")...)
	mbuf = append(mbuf, []byte("To: "+mailto+"\r\n")...)
	if mailres != "none" {
		// random mail boundary (crypto/rand not necessary)
		rand.Seed(t.UnixNano())
		boundbuf := make([]byte, 22)
		for i := range boundbuf {
			boundbuf[i] = encabc[rand.Intn(61)]
		}
		boundary = string(boundbuf)
		mbuf = append(mbuf, []byte(`Content-Type: multipart/mixed; boundary="`+boundary+`"`+"\r\n\r\n")...)
		mbuf = append(mbuf, []byte("--"+boundary+"\r\n")...)
	}
	mbuf = append(mbuf, []byte(`Content-Type: text/plain; charset="UTF-8"`+"\r\n\r\n")...)
	mbuf = append(mbuf, []byte("Event Type: "+eventtype+"\r\n")...)
	mbuf = append(mbuf, []byte("Date: "+t.Format(time.RFC1123)+"\r\n")...)
	if mailip == "on" {
		// get public ip from google / cloudflare
		var ip []string
		var err error
		gl := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Duration(5) * time.Second,
				}
				return d.DialContext(ctx, network, "ns1.google.com:53")
			},
		}
		ip, err = gl.LookupTXT(context.Background(), "o-o.myaddr.l.google.com")
		if err != nil {
			// on failure try cloudflare
			cf := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{
						Timeout: time.Duration(5) * time.Second,
					}
					return d.DialContext(ctx, network, "ns1.cloudflare.com:53")
				},
			}
			ip, err = cf.LookupTXT(context.Background(), "whoami.cloudflare.com")
		}
		if err != nil {
			mbuf = append(mbuf, []byte("\r\nRTSP Stream: error\r\n")...)
			logmsg(err.Error() + " (err 9)")
		} else if mailres == "low" {
			mbuf = append(mbuf, []byte("\r\nRTSP Stream: rtsp://"+ip[0]+":"+rtspport+"/ch0_1.h264\r\n")...)
		} else if mailres == "high" {
			mbuf = append(mbuf, []byte("\r\nRTSP Stream: rtsp://"+ip[0]+":"+rtspport+"/ch0_0.h264\r\n")...)
		}
	}
	if mailres != "none" {
		mbuf = append(mbuf, []byte("\r\n--"+boundary+"\r\n")...)
		mbuf = append(mbuf, []byte(`Content-Type: image/jpeg; name="Snapshot.jpg"`+"\r\n")...)
		mbuf = append(mbuf, []byte(`Content-Disposition: attachment; filename="Snapshot.jpg"`+"\r\n")...)
		mbuf = append(mbuf, []byte("Content-Transfer-Encoding: base64\r\n")...)
		mbuf = append(mbuf, []byte("X-Attachment-Id: file0\r\n\r\n")...)

		cmdg := exec.Command("/tmp/sd/yi-hack/bin/imggrabber", "-m", camsuf, "-r", mailres, "-w")
		sog, err := cmdg.StdoutPipe()
		if err != nil {
			logmsg(err.Error() + " (err 10)")
			mbuf = mbuf[:0]
			return err
		}

		if err := cmdg.Start(); err != nil {
			logmsg(err.Error() + " (err 11)")
			mbuf = mbuf[:0]
			return err
		}

		// base64 encoding with \r\n after 76 chars
		var v uint
		ic, il, ir, iv := 0, 0, 0, 0
		for {
			// snapshot in 570 byte chunks (lower B/op). Devices with more memory should use larger chunks (faster ns/op)
			if ir, _ = io.ReadFull(sog, rbuf); ir < 570 {
				// remaining lines
				for ic, iv = ir/3*3, 0; iv < ic; il, iv = il+1, iv+3 {
					v = uint(rbuf[iv])<<16 | uint(rbuf[iv+1])<<8 | uint(rbuf[iv+2])
					if il < 19 {
						mbuf = append(mbuf, encabc[v>>18&0x3F], encabc[v>>12&0x3F], encabc[v>>6&0x3F], encabc[v&0x3F])
					} else {
						mbuf = append(mbuf, 13, 10, encabc[v>>18&0x3F], encabc[v>>12&0x3F], encabc[v>>6&0x3F], encabc[v&0x3F])
						il = 0
					}
				}
				if il == 19 {
					mbuf = append(mbuf, 13, 10)
					il = 0
				}
				// remaining bytes
				if ir%3 == 1 {
					v = uint(rbuf[iv]) << 16
					mbuf = append(mbuf, encabc[v>>18&0x3F], encabc[v>>12&0x3F], 61, 61, 13, 10)
				} else if ir%3 == 2 {
					v = uint(rbuf[iv])<<16 | uint(rbuf[iv+1])<<8
					mbuf = append(mbuf, encabc[v>>18&0x3F], encabc[v>>12&0x3F], encabc[v>>6&0x3F], 61, 13, 10)
				} else {
					mbuf = append(mbuf, 13, 10)
				}
				// last msg line
				mbuf = append(mbuf, []byte("--"+boundary+"--")...)
				break
			}
			// full chunks
			for iv = 0; iv < 570; il, iv = il+1, iv+3 {
				v = uint(rbuf[iv])<<16 | uint(rbuf[iv+1])<<8 | uint(rbuf[iv+2])
				if il < 19 {
					mbuf = append(mbuf, encabc[v>>18&0x3F], encabc[v>>12&0x3F], encabc[v>>6&0x3F], encabc[v&0x3F])
				} else {
					mbuf = append(mbuf, 13, 10, encabc[v>>18&0x3F], encabc[v>>12&0x3F], encabc[v>>6&0x3F], encabc[v&0x3F])
					il = 0
				}
			}
		}

		if err := cmdg.Wait(); err != nil {
			logmsg(err.Error() + " (err 12)")
			mbuf = mbuf[:0]
			return err
		}
	}

	auth := smtp.PlainAuth("", mailusr, mailpw, mailhost)
	if err := smtp.SendMail(mailhost+":"+mailport, auth, mailfrom, []string{mailto}, mbuf); err != nil {
		logmsg(err.Error() + " (err 13)")
		mbuf = mbuf[:0]
		return err
	}

	// reset buffer
	mbuf = mbuf[:0]
	for i := 0; i < 570; i++ {
		rbuf[i] = 0
	}

	return nil
}
