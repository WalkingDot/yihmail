# Email notifications for [YI-Hack](https://github.com/roleoroleo/yi-hack-Allwinner-v2) cameras

<p align="center">
<img src="https://user-images.githubusercontent.com/67340594/168180894-ffae19b5-a113-4d0e-9a24-f81c14e9d1c6.jpg" width="340" height="400" alt="yihmail_preview">
</p>


Copy 	**yihmail** into this directory:  
`/tmp/sd/yi-hack/bin`

Edit the YI-Hack autostart:  
`/tmp/sd/yi-hack/startup.sh`

Add this example and restart your camera:  
`/tmp/sd/yi-hack/bin/yihmail -f YourAccount@domain.com -t Account@domain.com -u YourAccount@domain.com -p 123456 -h smtp.domain.com > /dev/null 2>&1 &`

```
Usage of yihmail:
yihmail -f -t -u -p -h [-n -m -r -i -w -s -v -z -e -l]
-f	Email from.
	example: YourAccount@domain.com
-t	Email to.
	example: Account@domain.com
-u	Account name.
	example: YourAccount@domain.com or only YourAccount
-p	Account password.
-h	SMTP host.
	example: smtp.domain.com or smtp-mail.domain.com
-n	SMTP port.
	(optional - default: 587)
-m	Run permanently in background or send only one email.
	If started as oneshot, exit status is 0 on success.
	options: daemon, oneshot (optional - default: daemon)
-r	Set the preferred resolution.
	options: low, high, none (optional - default: low)
-i	Get a RTSP link with public IP.
	options: on, off (optional - default: off)
-w	Waits in seconds between emails.
	options: 0s - 99999s (optional - default: 600s)
-s	To skip events choose initial letters (Motion, Sound, Human, Baby).
	options: mshb (optional)
	example for only motion events: shb
-v	RTSP port.
	(optional - default: 554)
-z	Time zone fix, if you have a wrong time.
	option hours: -23h to 23h (optional - default: 0h)
-e	Set a custom oneshot event name.
	(optional - default: none)
-l	Use log file; available at: http://IP-CAM:8080/log/mail.txt
	options: on, off (optional - default: off)
```
