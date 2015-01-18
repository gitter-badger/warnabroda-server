package routes

import (
	"bitbucket.org/hbtsmith/warnabrodagomartini/models"
	"bitbucket.org/hbtsmith/warnabrodagomartini/messages"
	"github.com/coopernurse/gorp"	
	"github.com/streadway/amqp"
	"os"
	"flag"
	"fmt"
	"log"
	"time"
	"encoding/json"

)

var (	
	RabbitMQUser 		= os.Getenv("WARNARABBITMQUSER")
	RabbitMQPass		= os.Getenv("WARNARABBITMQPASS")
	HostWarnabroda		= os.Getenv("WARNAHOST")
	uri          		= flag.String("uri", "amqp://"+RabbitMQUser+":"+RabbitMQPass+"@"+HostWarnabroda+":5672/", "AMQP URI")
	exchangeName 		= flag.String("exchange", "warnabroda-whatsapp-queue", "Durable AMQP exchange name")
	exchangeType 		= flag.String("exchange-type", "direct", "Exchange type - direct|fanout|topic|x-custom")
	routingKey   		= flag.String("key", "warnabroda-key", "AMQP routing key")	
	reliable     		= flag.Bool("reliable", true, "Wait for the publisher confirmation before exiting")
)

func init(){
	flag.Parse()
}

// For now all due verifications regarding send rules is done previewsly, here we just async the e-mail send of the warn
func ProcessWhatsapp(warning *models.Warning, db gorp.SqlExecutor){	
	
	go sendWhatsappWarn(warning, db)
	
}

//Deploys the message to be sent into an email struct, call the service and in case of successful send, update the warn as sent.
func sendWhatsappWarn(entity *models.Warning, db gorp.SqlExecutor) {	

	subject := GetRandomSubject(entity.Lang_key)
	message := SelectMessage(db, entity.Id_message)
	footer  := messages.GetLocaleMessage(entity.Lang_key,"MSG_FOOTER")
	whatsMsg := models.Whatsapp {
		Id: entity.Id,
		Number: entity.Contact,
		Message: subject.Name + " : "+message.Name + " "+footer,
	}
	whatsJson, _ := json.Marshal(whatsMsg)
	fmt.Println(whatsJson)
	body         := flag.String("body", string(whatsJson[:]), "JSON body message")
	flag.Parse()
	

	if err := publish(*uri, *exchangeName, *exchangeType, *routingKey, *body, *reliable); err != nil {
		log.Fatalf("%s", err)
	}
	log.Printf("published %dB OK", len(*body))
	

}

func publish(amqpURI, exchange, exchangeType, routingKey, body string, reliable bool) error {

	// This function dials, connects, declares, publishes, and tears down,
	// all in one go. In a real service, you probably want to maintain a
	// long-lived connection as state, and publish against that.

	//log.Printf("dialing %q", amqpURI)
	connection, err := amqp.Dial(amqpURI)
	if err != nil {
		return fmt.Errorf("Dial: %s", err)
	}
	defer connection.Close()

	//log.Printf("got Connection, getting Channel")
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("Channel: %s", err)
	}

	//log.Printf("got Channel, declaring %q Exchange (%q)", exchangeType, exchange)
	if err := channel.ExchangeDeclare(
		exchange,     // name
		exchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // noWait
		nil,          // arguments
	); err != nil {
		return fmt.Errorf("Exchange Declare: %s", err)
	}

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	if reliable {
		log.Printf("enabling publishing confirms.")
		if err := channel.Confirm(false); err != nil {
			return fmt.Errorf("Channel could not be put into confirm mode: %s", err)
		}

		ack, nack := channel.NotifyConfirm(make(chan uint64, 1), make(chan uint64, 1))

		defer confirmOne(ack, nack)
	}

	//log.Printf("declared Exchange, publishing %dB body (%q)", len(body), body)
	if err = channel.Publish(
		exchange,   // publish to an exchange
		routingKey, // routing to 0 or more queues
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "application/json",
			ContentEncoding: "",
			Body:            []byte(body),
			Timestamp:    	 time.Now(),
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return fmt.Errorf("Exchange Publish: %s", err)
	}

	return nil
}

// One would typically keep a channel of publishings, a sequence number, and a
// set of unacknowledged sequence numbers and loop until the publishing channel
// is closed.
func confirmOne(ack, nack chan uint64) {
	log.Printf("waiting for confirmation of one publishing")

	select {
	case tag := <-ack:
		log.Printf("confirmed delivery with delivery tag: %d", tag)
	case tag := <-nack:
		log.Printf("failed delivery of delivery tag: %d", tag)
	}
}