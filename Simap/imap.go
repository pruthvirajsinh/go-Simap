package synco

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

func CopyEmails(acct *IMAPAccount, src string, dst string, uids []uint32, jobSize int) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Copying for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server)
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

func MoveEmails(acct *IMAPAccount, src string, dst string, uids []uint32, jobSize int) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Starting Moving for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server)
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

	log.Printf("Moving: %d UIDs total, %d jobs of size <= %d to %s\n", len(uids), len(jobs), jobSize, dst)

	for _, jobUIDs := range jobs {
		log.Println("Moving ", jobUIDs)
		err1 := WaitResp(c.UIDCopy(jobUIDs, dst))
		if err1 != nil {
			log.Println(err)
			continue
		}

		err1 = WaitResp(c.UIDStore(jobUIDs, "+FLAGS.SILENT", imap.NewFlagSet(`\Deleted`)))
		if err1 != nil {
			log.Println(err)
			continue
		}

		err1 = WaitResp(c.Expunge(jobUIDs))

		if err1 != nil {
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

func DeleteEmails(acct *IMAPAccount, src string, dst string, uids []uint32) (err error) {

	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Running for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	jobsize := 250

	// Fetch UIDs.
	c, errD := Dial(acct.Server)
	if errD != nil {
		err = errD
		return
	}
	_, err = login(c, acct.Username, acct.Password)
	if err != nil {
		return
	}
	if src == "" {
		err = errors.New("No source provided")
		return
	}
	if dst == "" {
		err = errors.New("No Dst provided")
		return
	}
	_, err = c.Select(dst, true)
	if err != nil { //Doesnt Exist->Create
		_, err = c.Create(dst)
		if err != nil {
			return
		}
	}

	timestarted := time.Now()

	nparts := (len(uids) + jobsize - 1) / jobsize
	jobs := make([]*imap.SeqSet, nparts)
	for i := 0; i < nparts; i++ {
		lo := i * jobsize
		hi_exclusive := (i + 1) * jobsize
		if hi_exclusive >= len(uids) {
			hi_exclusive = len(uids) - 1
			for uids[hi_exclusive] == 0 { // hacky
				hi_exclusive--
			}
		}
		set, _ := imap.NewSeqSet(":")

		set.AddNum(uids[lo:hi_exclusive]...)
		jobs[i] = set
	}

	log.Printf("%d UIDs total, %d jobs of size <= %d\n", len(uids), len(jobs), jobsize)

	for _, jobUIDs := range jobs {
		c.Copy(jobUIDs, dst)
	}

	timeelapsed := time.Since(timestarted)
	msecpermessage := timeelapsed.Seconds() / float64(len(uids)) * 1000
	messagespersec := float64(len(uids)) / timeelapsed.Seconds()
	log.Printf("Finished copying %d messages in %.2fs (%.1fms per message; %.1f messages per second)\n", len(uids), timeelapsed.Seconds(), msecpermessage, messagespersec)

	_, err = c.Close(false)
	_, err = c.Logout(-1)
	return

}

func GetEMails(acct *IMAPAccount, query string, mbox string, jobSize int) (mails []MsgData, err error) {
	imap.DefaultLogger = log.New(os.Stdout, "", 0)
	//	imap.DefaultLogMask = imap.LogConn | imap.LogRaw

	log.Printf("Running for user '%s' on IMAP server '%s:%d'", acct.Username, acct.Server.Host, acct.Server.Port)

	if jobSize <= 0 {
		jobSize = 10
	}

	// Fetch UIDs.
	c, errD := Dial(acct.Server)
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
		fmt.Println("Error while Search UID: ", err1)
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
	fmt.Println("UID Searching with querry ", query)
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
			fmt.Println(rsp.MessageInfo().Attrs["UID"])

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
		log.Println(uid, ":TXT", err1)
	}

	if b, err2 := HTMLBody(msg); err2 == nil {
		msgData.HtmlBody = b
	} else {
		log.Println(uid, ":HTML", err2)
	}

	if b, err3 := GpgBody(msg); err3 == nil {
		msgData.GpgBody = b
	} else {
		log.Println(uid, ":GPG", err3)
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

func Dial(server *IMAPServer) (c *imap.Client, err error) {

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	config := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         server.Host,
	}
	c, err = imap.DialTLS(addr, config)
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
