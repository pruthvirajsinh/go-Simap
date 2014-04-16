//Package main implements command line Example to check the functionalities of the Package
package main

import (
	"flag"
	"fmt"
	"github.com/pruthvirajsinh/go-Simap/Simap"
	"os"
	"strconv"
)

var query = flag.String("query", "after:2012/09/12", "query to limit fetch")
var mbox = flag.String("mbox", "inbox", "name of mail box/folder from which you want to get mail")
var destBox = flag.String("dbox", "", "name of mail box/folder where you want to move mail")
var jobSize = flag.Int("jobsize", 2, "Number of Emails to be processed at a time")
var move = flag.Bool("move", false, "Weateher to move or copy the mails while dbox is given")
var del = flag.Bool("delete", false, "Just Delete the fetched mails.Just to check Delete Functionality.")
var skipCerti = flag.Bool("skipCerti", false, "If your IMAP server uses self signed certi then make this true to skip Certification verification.")
var imapFlag = flag.String("imapflag", "", "Flag the emails")

func usage() {
	fmt.Fprintf(os.Stderr, "usage: main [server] [port] [username] [password]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

//Example
//To Copy all the Read mails since 1st,April 2014 fom inbox to processed.
//./main --skipCerti=false --query="SINCE 01-APR-2014 SEEN" --mbox=inbox --dbox=processed imap.gmail.com 993 user@gmail.com supersecretpassword

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) != 4 {
		usage()
	}
	portI, _ := strconv.Atoi(args[1])
	port := uint16(portI)
	server := &Simap.IMAPServer{args[0], port}
	acct := &Simap.IMAPAccount{args[2], args[3], server}

	mails, err := Simap.GetEMails(acct, *query, *mbox, *jobSize, *skipCerti)
	if err != nil {
		fmt.Println("Error while Getting mails ", err)
		return
	}
	var uids []uint32
	fmt.Println("Fetched Mails ", len(mails))
	fmt.Println("UID		|	From		|	To		|		Subject		|Body	| HTMLBODY	|GPGBody")
	for _, msg := range mails {
		//PRocess Emails here
		errP := processEmail(msg)
		if errP != nil {
			continue
		}
		//If successfull then append them to be moved to processed
		uids = append(uids, msg.Imap_uid)
	}
	if *imapFlag != "" {
		err = Simap.MarkEmails(acct, *mbox, *imapFlag, uids, *jobSize, *skipCerti)
		if err != nil {
			fmt.Println("Main : Error while Marking ", err)
		}
	}

	if *del == true {
		err = Simap.DeleteEmails(acct, *mbox, uids, *jobSize, *skipCerti)
		if err != nil {
			fmt.Println("Main : Error while Deleting ", err)
		}
		return

	}

	if *destBox != "" {
		if *move == true {
			err = Simap.MoveEmails(acct, *mbox, *destBox, uids, *jobSize, *skipCerti)
			if err != nil {
				fmt.Println("Eror while moving ", err)
			}
		} else {
			err = Simap.CopyEmails(acct, *mbox, *destBox, uids, *jobSize, *skipCerti)
			if err != nil {
				fmt.Println("Eror while Copying ", err)
			}
		}
	}

}

func processEmail(msg Simap.MsgData) (err error) {
	fmt.Println("[" + strconv.Itoa(int(msg.Imap_uid)) + "]  |  " + msg.From + "  |  " + msg.To + "  |  " + msg.Subject + "  |  " +
		msg.Body + " | " + msg.HtmlBody + " | " + msg.GpgBody)
	return
}
