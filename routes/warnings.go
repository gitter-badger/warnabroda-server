package routes

import (
	"bitbucket.org/hbtsmith/warnabrodagomartini/models"
	"fmt"
	"github.com/coopernurse/gorp"
	"github.com/go-martini/martini"
	"github.com/mostafah/mandrill"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var wab_root = os.Getenv("WARNAROOT")

//https://www.facilitamovel.com.br/api/simpleSend.ft?user=warnabroda&password=superwarnabroda951753&destinatario=4896662015&msg=WarnabrodaTest
func sendEmail(entity *models.Warning, db gorp.SqlExecutor) {

	mandrill.Key = os.Getenv("MANDRILL_KEY")
	// you can test your API key with Ping
	err := mandrill.Ping()
	// everything is OK if err is nil

	wab_email_template := wab_root + "/models/warning.html"

	//reads the e-mail template from a local file
	template_byte, err := ioutil.ReadFile(wab_email_template)
	checkErr(err, "File Opening ERROR")
	template_email_string := string(template_byte[:])

	//Get a random subject for the e-mails subject
	subject := GetRandomSubject()

	//Get the label for the sending warning
	message := SelectMessage(db, entity.Id_message)

	var email_content string
	email_content = strings.Replace(template_email_string, "{{warning}}", message.Name, 1)

	msg := mandrill.NewMessageTo(entity.Contact, subject.Name)
	msg.HTML = email_content
	// msg.Text = "plain text content" // optional
	msg.Subject = subject.Name
	msg.FromEmail = os.Getenv("WARNAEMAIL")
	msg.FromName = "Warn A Broda: Dá um toque"

	//envio assincrono = true // envio sincrono = false
	res, err := msg.Send(false)

	if res[0] != nil {
		UpdateWarningSent(entity, db)
	} else {
		fmt.Println(res[0])
	}

}

func sendSMS(entity *models.Warning, db gorp.SqlExecutor) {
	message := SelectMessage(db, entity.Id_message)
	sms_message := "Ola Amigo(a), "
	sms_message += "Você " + message.Name + ". "
	sms_message += "Avise um amigo você também: www.warnabroda.com"
	u, err := url.Parse("https://www.facilitamovel.com.br/api/simpleSend.ft?")

	if err != nil {
		checkErr(err, "Ugly URL")
	}
	u.Scheme = "https"
	u.Host = "www.facilitamovel.com.br"
	q := u.Query()
	q.Set("user", os.Getenv("WARNASMS_USER"))
	q.Set("password", os.Getenv("WARNASMS_PASS"))
	q.Set("destinatario", entity.Contact)
	q.Set("msg", sms_message)
	u.RawQuery = q.Encode()

	res, err := http.Get(u.String())
	if err != nil {
		checkErr(err, "SMS Not Sent")
	}
	robots, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		checkErr(err, "No response from SMS Sender")
	} else {
		entity.Message = string(robots[:])
		UpdateWarningSent(entity, db)
	}

}

func GetWarnings(enc Encoder, db gorp.SqlExecutor) (int, string) {
	var warnings []models.Warning
	_, err := db.Select(&warnings, "select * from warnings order by id")
	if err != nil {
		checkErr(err, "select failed")
		return http.StatusInternalServerError, ""
	}
	return http.StatusOK, Must(enc.Encode(warningsToIface(warnings)...))
}

func GetWarning(enc Encoder, db gorp.SqlExecutor, parms martini.Params) (int, string) {
	id, err := strconv.Atoi(parms["id"])
	obj, _ := db.Get(models.Warning{}, id)
	if err != nil || obj == nil {
		checkErr(err, "get failed")
		// Invalid id, or does not exist
		return http.StatusNotFound, ""
	}
	entity := obj.(*models.Warning)
	return http.StatusOK, Must(enc.EncodeOne(entity))
}

func AddWarning(entity models.Warning, w http.ResponseWriter, enc Encoder, db gorp.SqlExecutor) (int, string) {

	status := &models.Message{
		Id:       200,
		Name:     "Broda Avisado(a)",
		Lang_key: "br",
	}

	entity.Sent = false
	entity.Created_by = "system"
	entity.Created_date = models.JDate(time.Now().Local())
	entity.Lang_key = "br"

	err := db.Insert(&entity)
	if err != nil {
		checkErr(err, "insert failed")
		return http.StatusConflict, ""
	}
	w.Header().Set("Location", fmt.Sprintf("/warnabroda/warnings/%d", entity.Id))

	if entity.Id_contact_type == 1 {
		go sendEmail(&entity, db)
	} else if entity.Id_contact_type == 2 {
		processSMS(&entity, db, status)

	}

	return http.StatusCreated, Must(enc.EncodeOne(status))
}

func processSMS(warning *models.Warning, db gorp.SqlExecutor, status *models.Message) {

	if smsSentToContact(warning, db) {
		status.Id = 403
		status.Name = "Este número já recebeu um SMS hoje ou seu IP(" + warning.Ip + ") já enviou a cota maxima de SMS diário."
		status.Lang_key = "br"
	} else {
		go sendSMS(warning, db)
	}

}

func smsSentToContact(warning *models.Warning, db gorp.SqlExecutor) bool {

	str_today := time.Now().Format("2006-01-02")

	return_statement := false
	var warnings []models.Warning

	select_statement := " SELECT * FROM warnings "
	select_statement += " WHERE Id_contact_type = 2 AND "
	select_statement += " (Contact = '" + warning.Contact + "' OR Ip LIKE '%" + warning.Ip + "%' ) AND "
	select_statement += " Created_date BETWEEN '" + str_today + " 00:00:00' AND '" + str_today + " 23:59:59' AND "
	select_statement += " Id <> " + strconv.FormatInt(warning.Id, 10)

	_, err := db.Select(&warnings, select_statement)
	if err != nil {
		checkErr(err, "Checking Contact failed")
	}

	if len(warnings) > 0 {
		return_statement = true
	}

	return return_statement
}

func UpdateWarningSent(entity *models.Warning, db gorp.SqlExecutor) {
	entity.Sent = true
	entity.Last_modified_date = models.JDate(time.Now())
	_, err := db.Update(entity)
	if err != nil {
		checkErr(err, "update failed")
	}
}

func UpdateWarning(entity models.Warning, enc Encoder, db gorp.SqlExecutor, parms martini.Params) (int, string) {
	id, err := strconv.Atoi(parms["id"])
	obj, _ := db.Get(models.Warning{}, id)
	if err != nil || obj == nil {
		checkErr(err, "get failed")
		// Invalid id, or does not exist
		return http.StatusNotFound, ""
	}
	oldEntity := obj.(*models.Warning)

	entity.Id = oldEntity.Id
	_, err = db.Update(&entity)
	if err != nil {
		checkErr(err, "update failed")
		return http.StatusConflict, ""
	}
	return http.StatusOK, Must(enc.EncodeOne(entity))
}

func DeleteWarning(db gorp.SqlExecutor, parms martini.Params) (int, string) {
	id, err := strconv.Atoi(parms["id"])
	obj, _ := db.Get(models.Warning{}, id)
	if err != nil || obj == nil {
		checkErr(err, "get failed")
		// Invalid id, or does not exist
		return http.StatusNotFound, ""
	}
	entity := obj.(*models.Warning)
	_, err = db.Delete(entity)
	if err != nil {
		checkErr(err, "delete failed")
		return http.StatusConflict, ""
	}
	return http.StatusNoContent, ""
}

func warningsToIface(v []models.Warning) []interface{} {
	if len(v) == 0 {
		return nil
	}
	ifs := make([]interface{}, len(v))
	for i, v := range v {
		ifs[i] = v
	}
	return ifs
}
