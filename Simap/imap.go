//Package Simap implements a simple Imap client which
//--Fetches,Copies,Moves emails from mailboxes.
//--Creates,Deletes Mboxes/Folders on the Server.
//--Marks,Unmarks Imap Flags from the mails.
//--Can Skip Certificate Verification of the IMAP Server. (Good for IMAP servers using SelfSigned Cerificates.)
package Simap

import "code.google.com/p/go-imap/go1/imap"
import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"time"
)

type IMAPServer struct {
	Host string
	Port uint16
}

type IMAPAccount struct {
	Username string
	Password string
	Server   *IMAPServer
}

type UIDFetchJob struct {
	uids []uint32
}

type MsgData struct {
	Imap_uid uint32
	From     string
	To       string
	Subject  string
	Body     string
	HtmlBody string
	GpgBody  string
	Header   mail.Header
}

func WaitResp(cmd *imap.Command, err error) error {
	if err == nil {
		for cmd.InProgress() {
			cmd.Client().Recv(-1)
			for _, rsp := range cmd.Data {
				log.Println(cmd.Name(true), " Response:", rsp.String())
			}
			cmd.Data = nil
		}
	}
	return err
}

//CreateMbox creates a mailbox/folder on the server
//If already exists then do nothing
//IF skipCerti is true then it will not check for the validity of the Certificate of the IMAP server,
//good only if IMAP server is using self signed certi.
func CreateMbox(acct *IMAPAccount, name string, skipCerti bool) (err error) {

	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	err = WaitResp(c.Select(name, false))
	if err != nil { //Doesnt Exist->Create
		err = WaitResp(c.Create(name))
		if err != nil {
			return
		}
		err = WaitResp(c.Select(name, false))
	}
	err = WaitResp(c.Close(false))
	return
}

//DeleteMbox deletes a mailbox/folder on the server
//If does not exist then do nothing
func DeleteMbox(acct *IMAPAccount, name string, skipCerti bool) (err error) {

	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	err = WaitResp(c.Select(name, false))
	if err != nil { //Doesnt Exist,Return
		return
	}
	err = WaitResp(c.Close(false))
	if err != nil {
		return
	}
	err = WaitResp(c.Delete(name))
	return
}

//CopyEmails copies Emails having unique identifiers uids (NOT sequence numbers) of mailbox src
//to mailbox dst by making bunches of jobsize.
//If dst mbox doesnt exist then it will create it.
//IF skipCerti is true then it will not check for the validity of the Certificate of the IMAP server,
//good only if IMAP server is using self signed certi.
//e.g. if jobsize =10 and total emails =100 then it will create 10 bunches of size 10 and then copy them.
func CopyEmails(acct *IMAPAccount, src string, dst string, uids []uint32, jobSize int, skipCerti bool) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Copying for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	if src == "" {
		err = errors.New("No source provided")
		return
	}
	if dst == "" {
		err = errors.New("No Dst provided")
		return
	}

	//PRC Dest MBOX CHeck Start
	err = WaitResp(c.Select(dst, false))
	if err != nil { //Doesnt Exist->Create
		err = WaitResp(c.Create(dst))
		if err != nil {
			return
		}
		err = WaitResp(c.Select(dst, false))
	}
	err = WaitResp(c.Close(false))
	if err != nil {
		return
	}
	//PRC Dest MBOX Check End

	err = WaitResp(c.Select(src, true))
	if err != nil {
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {
		/*Logic
		1.If we are at last index len(uids)-1 and still have not reached jobSize limit then append that to jobs
			(In other words if set size is smaller then jobsize)
		2.if we go over index of size greater then jobsize then add all elemetns up to that index in to jobs
		3.0 mod n returns 0 hence i!=0 is checked

		*/
		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("Copying: %d UIDs total, %d jobs of size <= %d to %s\n", len(uids), len(jobs), jobSize, dst)

	for _, jobUIDs := range jobs {
		log.Println("Copying ", jobUIDs)
		err1 := WaitResp(c.UIDCopy(jobUIDs, dst))
		if err1 != nil {
			log.Println(err)
			continue
		}

	}
	err = WaitResp(c.Close(false))
	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished copying %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	return

}

//MoveEmails is similar to CopyEmails but it moves the mails to dst hence deleting mails from src.
func MoveEmails(acct *IMAPAccount, src string, dst string, uids []uint32, jobSize int, skipCerti bool) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Moving for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	if src == "" {
		err = errors.New("No source provided")
		return
	}
	if dst == "" {
		err = errors.New("No Dst provided")
		return
	}

	//PRC Dest MBOX CHeck Start
	err = WaitResp(c.Select(dst, false))
	if err != nil { //Doesnt Exist->Create
		err = WaitResp(c.Create(dst))
		if err != nil {
			return
		}
		err = WaitResp(c.Select(dst, false))
	}
	err = WaitResp(c.Close(false))
	if err != nil {
		return
	}
	//PRC Dest MBOX Check End

	err = WaitResp(c.Select(src, false))
	if err != nil {
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {

		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("Moving: %d UIDs total, %d jobs of size <= %d to %s\n", len(uids), len(jobs), jobSize, dst)

	for _, jobUIDs := range jobs {
		log.Println("Moving ", jobUIDs)
		err1 := WaitResp(c.UIDCopy(jobUIDs, dst))
		if err1 != nil {
			log.Println(err)
			continue
		}

		err = WaitResp(c.UIDStore(jobUIDs, "+FLAGS.SILENT", imap.NewFlagSet(`\Deleted`)))
		if err != nil {
			log.Println(err)
			continue
		}

		err = WaitResp(c.Expunge(jobUIDs))

		if err != nil {
			log.Println("Expunge:", err)
			continue
		}

	}
	err = WaitResp(c.Close(false))
	if err != nil {
		log.Println("Error While Closing ", err)
		return
	}
	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished Moving %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	return

}

//DeleteEMails deletes mails having uids from src.Arguments have same meaning as CopyEmails
func DeleteEmails(acct *IMAPAccount, src string, uids []uint32, jobSize int, skipCerti bool) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Deleting for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	if src == "" {
		err = errors.New("No source provided")
		return
	}

	err = WaitResp(c.Select(src, false))
	if err != nil {
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {

		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("Deleting: %d UIDs total, %d jobs of size <= %d from %s\n", len(uids), len(jobs), jobSize, src)

	for _, jobUIDs := range jobs {
		log.Println("Deleting ", jobUIDs)

		err = WaitResp(c.UIDStore(jobUIDs, "+FLAGS.SILENT", imap.NewFlagSet(`\Deleted`)))
		if err != nil {
			log.Println(err)
			continue
		}

		err = WaitResp(c.Expunge(jobUIDs))

		if err != nil {
			log.Println("Expunge:", err)
			continue
		}

	}
	err = WaitResp(c.Close(false))
	if err != nil {
		log.Println("Error While Closing ", err)
		return
	}
	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished Deleting %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	return

}

//MarkEmails marks mails having uids from src with IMAP flag specified in imapFlag.
//See RFC 3501 section 2.3.2 for a list of all valid flags.
func MarkEmails(acct *IMAPAccount, src string, imapFlag string, uids []uint32, jobSize int, skipCerti bool) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Marking for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	if src == "" {
		err = errors.New("No source provided")
		return
	}

	err = WaitResp(c.Select(src, false))
	if err != nil {
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {

		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("Marking: %d UIDs with %s total, %d jobs of size <= %d from %s\n", len(uids), imapFlag, len(jobs), jobSize, src)

	for _, jobUIDs := range jobs {
		log.Println("Marking ", jobUIDs)

		err = WaitResp(c.UIDStore(jobUIDs, "+FLAGS.SILENT", imap.NewFlagSet(imapFlag)))
		if err != nil {
			log.Println(err)
			continue
		}

	}
	err = WaitResp(c.Close(false))
	if err != nil {
		log.Println("Error While Closing ", err)
		return
	}
	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished Marking %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	return

}

//UnmarkEmails unmarks/resets flags of mails having uids from src with IMAP flag specified in imapFlag.
//See RFC 3501 section 2.3.2 for a list of all valid flags.
func UnMarkEmails(acct *IMAPAccount, src string, imapFlag string, uids []uint32, jobSize int, skipCerti bool) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting UnMarking for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}

	defer c.Logout(-1)

	if src == "" {
		err = errors.New("No source provided")
		return
	}

	err = WaitResp(c.Select(src, false))
	if err != nil {
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {

		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("UnMarking: %d UIDs with %s total, %d jobs of size <= %d from %s\n", len(uids), imapFlag, len(jobs), jobSize, src)

	for _, jobUIDs := range jobs {
		log.Println("UnMarking ", jobUIDs)

		err = WaitResp(c.UIDStore(jobUIDs, "-FLAGS.SILENT", imap.NewFlagSet(imapFlag)))
		if err != nil {
			log.Println(err)
			continue
		}

	}
	err = WaitResp(c.Close(false))
	if err != nil {
		log.Println("Error While Closing ", err)
		return
	}
	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished UnMarking %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	return

}

//GetEmails gets Emails from mailbox mbox in Struct of Type MsgData.
//It searches the mailbox for messages that match the given searching criteria mentioned in query string.
//See RFC 3501 section 6.4.4 for a list of all valid search keys.
//It is the caller's responsibility to quote strings when necessary.
//All strings must use UTF-8 encoding.
func GetEMails(acct *IMAPAccount, query string, mbox string, jobSize int, skipCerti bool) (mails []MsgData, err error) {
	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Running for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server, skipCerti)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}
	defer c.Logout(-1)
	if mbox == "" {
		mbox = "inbox"
	}
	err = WaitResp(c.Select(mbox, true))
	if err != nil {
		return
	}
	uids, err1 := SearchUIDs(c, query)
	if err1 != nil {
		err = err1
		return
	}

	timestarted := time.Now()

	var jobs []*imap.SeqSet

	var jUids []uint32
	for i := 0; i < len(uids); i++ {
		/*Logic
		1.If we are at last index len(uids)-1 and still have not reached jobSize limit then append that to jobs
			(In other words if set size is smaller then jobsize)
		2.if we go over index of size greater then jobsize then add all elemetns up to that index in to jobs
		3.0 mod n returns 0 hence i!=0 is checked

		*/
		if i%(jobSize) == 0 && i != 0 { //Append the new job to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
		jUids = append(jUids, uids[i])
		if i == len(uids)-1 { //Last Element Encountered Add to jobs
			set, _ := imap.NewSeqSet("")
			set.AddNum(jUids[:]...)
			jobs = append(jobs, set)
			jUids = nil
		}
	}

	log.Printf("%d UIDs total, %d jobs of size <= %d\n", len(uids), len(jobs), jobSize)

	for _, jobUIDs := range jobs {
		fetched, errF := FetchMessages(c, jobUIDs)
		if errF != nil {
			log.Println("error while fetching ", jobUIDs, " ", err)
			continue
		}
		for _, msg := range fetched {
			mails = append(mails, msg)
		}
	}

	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished fetching %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	err = WaitResp(c.Close(false))
	return
}

func SearchUIDs(c *imap.Client, query string) (uids []uint32, err error) {
	//cmd, err := c.UIDSearch("X-GM-RAW", fmt.Sprint("\"", query, "\""))
	//cmd, err := c.UIDSearch(fmt.Sprint("\"", query, "\""))
	cmd, err := c.UIDSearch(query)
	if err != nil {
		return
	}
	for cmd.InProgress() {
		c.Recv(-1)
		for _, rsp := range cmd.Data {
			uids = rsp.SearchResults()
		}
		cmd.Data = nil
	}
	return
}

func FetchAllUIDs(c *imap.Client) (uids []uint32, err error) {
	maxmessages := 150000
	uids = make([]uint32, maxmessages)

	set, errS := imap.NewSeqSet("1:*")
	if errS != nil {
		err = errS
		return
	}

	cmd, errF := c.UIDFetch(set, "RFC822.SIZE")
	if errF != nil {
		err = errF
		return
	}

	messagenum := uint32(0)
	for cmd.InProgress() {
		errC := c.Recv(-1)
		if errC != nil {
			continue
		}
		for _, rsp := range cmd.Data {
			uid := imap.AsNumber(rsp.MessageInfo().Attrs["UID"])
			uids[messagenum] = uid
		}
		cmd.Data = nil
		messagenum++
	}

	uids = uids[:messagenum]
	return
}

func FetchMessages(c *imap.Client, uidSet *imap.SeqSet) (fetched []MsgData, err error) {
	cmd, errF := c.UIDFetch(uidSet, "RFC822")
	if errF != nil {
		err = errF
		return
	}

	for cmd.InProgress() {
		errC := c.Recv(-1)
		if errC != nil {
			return
		}
		for _, rsp := range cmd.Data {

			uid := imap.AsNumber(rsp.MessageInfo().Attrs["UID"])
			mime := imap.AsBytes(rsp.MessageInfo().Attrs["RFC822"])
			msg, errR := mail.ReadMessage(bytes.NewReader(mime))
			if errR != nil {
				continue
			}
			if msg != nil {
				msgdata := GetMessage(msg, uid)
				fetched = append(fetched, msgdata)
			}
		}
		cmd.Data = nil
	}

	return
}

func GetMessage(msg *mail.Message, uid uint32) (msgData MsgData) {

	msgData.Header = msg.Header

	msgData.From = msg.Header.Get("From")
	msgData.To = msg.Header.Get("To")
	msgData.Subject = msg.Header.Get("Subject")
	msgData.Imap_uid = uid

	if b, err1 := TextBody(msg); err1 == nil {
		msgData.Body = b
	} else {
		//log.Println(uid, ":TXT", err1)
	}

	if b, err2 := HTMLBody(msg); err2 == nil {
		msgData.HtmlBody = b
	} else {
		//log.Println(uid, ":HTML", err2)
	}

	if b, err3 := GpgBody(msg); err3 == nil {
		msgData.GpgBody = b
	} else {
		//log.Println(uid, ":GPG", err3)
	}

	return
}

func GetMessageAsJSON(msg MsgData) (msgJSON string, err error) {

	var msgdata = map[string]string{}

	for headerkey := range msg.Header {
		val := msg.Header.Get(headerkey)
		msgdata[headerkey] = val
	}

	msgdata["imap_uid"] = fmt.Sprintf("%d", msg.Imap_uid)

	if msg.Body != "" {
		msgdata["text_body"] = msg.Body
	}
	if msg.HtmlBody != "" {
		msgdata["html_body"] = msg.HtmlBody
	}
	if msg.GpgBody != "" {
		msgdata["gpg_body"] = msg.HtmlBody
	}

	o, err3 := json.Marshal(msgdata)
	if err3 != nil {
		log.Println("error marshaling message as JSON: ", err3.Error()[:100])
		err = err3
		return
	} else {
		//fmt.Println(string(o))
		msgJSON = string(o)
	}
	return
}

//Dial dials a connection to the IMAP server
//If skipCerti is true then it will not check for the validity of the Certificate of the IMAP server,
//good only if IMAP server is using self signed certi.
func Dial(server *IMAPServer, skipCerti bool) (c *imap.Client, err error) {

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	if skipCerti {
		config := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         server.Host,
		}
		c, err = imap.DialTLS(addr, config)
	} else {
		c, err = imap.DialTLS(addr, nil)
	}
	return
}

func login(c *imap.Client, user, pass string) (cmd *imap.Command, err error) {
	defer c.SetLogMask(sensitive(c, "LOGIN"))
	return c.Login(user, pass)
}

func sensitive(c *imap.Client, action string) imap.LogMask {
	mask := c.SetLogMask(imap.LogConn)
	hide := imap.LogCmd | imap.LogRaw
	if mask&hide != 0 {
		c.Logln(imap.LogConn, "Raw logging disabled during", action)
	}
	c.SetLogMask(mask &^ hide)
	return mask
}
