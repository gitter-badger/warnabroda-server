package routes

import (
	"bitbucket.org/hbtsmith/warnabrodagomartini/models"	
	"github.com/coopernurse/gorp"
	"io/ioutil"
	"net/http"
	"net/url"
)

func CaptchaResponse(captcha models.Captcha, w http.ResponseWriter, enc Encoder, db gorp.SqlExecutor) (int, string) {
	u, err := url.Parse("https://www.google.com/recaptcha/api/siteverify?")
	checkErr(err, "Ugly URL")	

	u.Scheme = "https"
	u.Host = "www.google.com"
	q := u.Query()
	q.Set("secret", "6LfcKP8SAAAAADK2RSEiv5PBQrNSrfimkynWfaPd")
	q.Set("response", captcha.Response)
	q.Set("remoteip", captcha.Ip)	
	q.Set("v", "php_1.0")	
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())	
	checkErr(err, "SMS Not Sent")
	
	robots, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	checkErr(err, "No response from SMS Sender")
	
	
	return http.StatusOK, string(robots[:])
}