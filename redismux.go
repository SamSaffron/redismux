package main

import (
	"flag"
	"fmt"
	"github.com/fzzy/radix/redis"
	"io/ioutil"
	"log"
	"net"
	// "net/http"
	// _ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var Version = "1.0.0"

var address = flag.String("listen", "0.0.0.0:6379", "The address the redis mux server listens on")
var verbose = flag.Bool("verbose", false, "Verbose output")
var profile = flag.String("profile", "", "write cpu profile to file")
var maxConnections = flag.Uint64("max_connections", 10000, "maximum number of open connection, will attempt to raise ulimit to this number")
var backupRedis = flag.String("backup_redis", "", "Address of backup redismux")
var masterRedis = flag.String("master_redis", "", "Address of master redismux")
var dbs = flag.String("db_dir", "dbs", "Directory for storage of all dbs")
var startChildrenOnBoot = flag.Bool("start_children_on_boot", false, "Start child redis processes on boot")

var backupCheckMutex sync.Mutex

type RedisCommand struct {
	ParamCount int
	Params     []string
	Inline     bool
}

type RedisInfo struct {
	Pid          int
	Name         string
	Cmd          *exec.Cmd
	BackupPinged bool
	Done         chan bool
}

var redises = struct {
	sync.RWMutex
	m map[string]*RedisInfo
}{m: make(map[string]*RedisInfo)}

func main() {

	if os.Args[1] == "--version" {
		fmt.Println("redismux version " + Version)
		return
	}

	registerExitHandler()
	raiseUlimit()

	flag.Parse()

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	if *startChildrenOnBoot {
		files, _ := ioutil.ReadDir(*dbs)
		for _, f := range files {
			if f.IsDir() {
				startupRedis(f.Name())
			}
		}
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", *address)
	checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)

	// for pprof
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// error handling

		} else {
			go handleConnection(conn)
		}

	}
}

func raiseUlimit() {

	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Failed to get open file limit")
	}

	log.Println("Initial open file limit ", rLimit.Max, "/", rLimit.Cur)

	if rLimit.Max < *maxConnections {
		rLimit.Max = *maxConnections
	}

	if rLimit.Cur < *maxConnections {
		rLimit.Cur = *maxConnections
	}

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)

	if err != nil {
		log.Fatal("Failed to raise connection limit to ", maxConnections)
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Failed to get open file limit")
	}

	log.Println("New open file limit ", rLimit.Max, "/", rLimit.Cur)

}

func Verbose(v ...interface{}) {
	if *verbose {
		log.Println(v)
	}
}

func (redis RedisInfo) Stop() {

	pidString := strconv.Itoa(redis.Cmd.Process.Pid)

	log.Println("Stopping Redis " + redis.Name + " pid: " + pidString)

	timeout := make(chan bool)
	done := make(chan bool)

	redis.Cmd.Process.Signal(syscall.SIGTERM)

	go func() {
		time.Sleep(10 * time.Second)
		timeout <- true
	}()

	select {
	case <-redis.Done:
		log.Println("Redis ", redis.Name, "terminated gracefully ("+pidString+")")
	case <-timeout:
		redis.Cmd.Process.Signal(syscall.SIGKILL)
		log.Println("Redis ", redis.Name, "termination forced ("+pidString+")")
		<-done
	}
}

func watchRunningRedis(redis *RedisInfo) {
	err := redis.Cmd.Wait()
	if err != nil {
		log.Println(fmt.Errorf("Failed to wait on %v %v", redis.Name, err))
	} else {
		log.Println("Redis Stopped ", redis.Name)
		redis.Done <- true
	}
}

func ensureBackupRedis(ri *RedisInfo) {

	backupCheckMutex.Lock()
	defer backupCheckMutex.Unlock()

	if ri.BackupPinged {
		return
	}

	// first lets make sure we are running
	log.Println("ensuring backup redis exists for " + ri.Name)
	c, err := redis.DialTimeout("tcp", *backupRedis, time.Duration(10)*time.Second)
	if err != nil {
		log.Println("FAILURE: failed to connect to backup redis " + *backupRedis)
		return
	}

	r := c.Cmd("AUTH", ri.Name)
	if r.Err != nil {
		log.Println("FAILURE: failed to provision backup redis " + *backupRedis)
	} else {
		ri.BackupPinged = true
	}
}

func startupRedis(name string) {

	redises.Lock()
	defer redises.Unlock()

	_, alreadyStarted := redises.m[name]
	if alreadyStarted {
		return
	}

	log.Println("Starting up:", name)
	os.MkdirAll(*dbs+"/"+name, 0770)

	cmd := exec.Command("bin/redis-server",
		"config/redis.conf",
		"--unixsocket", "redis.sock",
		"--dir", "./"+*dbs+"/"+name+"/",
	)

	if len(*masterRedis) > 0 {
		cmd.Args = append(cmd.Args,
			"--slaveof "+strings.Replace(*masterRedis, ":", " ", -1),
			"--masterauth "+name)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()

	if err != nil {
		log.Println("FAILURE: failed to startup redis " + name)
		log.Println(err)
		return
	}

	info := &RedisInfo{
		Pid:  cmd.Process.Pid,
		Name: name,
		Cmd:  cmd,
		Done: make(chan bool),
	}

	go watchRunningRedis(info)

	redises.m[info.Name] = info

	if len(*backupRedis) > 0 {
		go ensureBackupRedis(info)
	}
}

func parse(buffer []byte, length int) (command RedisCommand, err error) {
	command = RedisCommand{}

	const (
		START = iota
		PARAMS
		LENGTH
		DATA
	)

	if length < 1 {
		err = fmt.Errorf("EMPTY COMMAND")
		return
	}

	// log.Println(string(buffer[:length]))

	command.Inline = string(buffer[0]) != "*"

	// special handling for inline
	if command.Inline {
		str := string(buffer[:length])
		split := strings.Split(strings.Trim(str, "\r\n"), " ")
		command.Params = split
		command.ParamCount = len(split)
		return
	}

	command.Inline = false

	state := START
	first := -1
	paramLength := -1
	pos := 0

	for i := 0; i < length; i++ {
		char := buffer[i]
		if char == '*' && state == START {
			first = i + 1
			state = PARAMS
		} else if state == PARAMS && char == '\r' {
			val := string(buffer[first:i])
			parsed, _ := strconv.Atoi(val)
			command.ParamCount = parsed
			command.Params = make([]string, parsed)
			state = START
		} else if char == '$' && state == START {
			first = i + 1
			state = LENGTH
		} else if state == LENGTH && char == '\r' {
			val := string(buffer[first:i])
			paramLength, _ = strconv.Atoi(val)
			state = DATA
		} else if state == DATA && char == '\n' {
			to := i + paramLength + 1
			if to > length {
				// NOT SUPPORTED ATM
				err = fmt.Errorf("NOT SUPPORTING LARGE COMMANDS YET")
				return
			}

			if pos >= len(command.Params) {
				err = fmt.Errorf("CORRUPT STREAM, NOT PARSING")
				return
			}
			command.Params[pos] = string(buffer[i+1 : to])
			pos++
			state = START
			i += paramLength + 2
		}

	}

	return
}

func glue(client net.Conn, server net.Conn, done chan bool) {

	var buffer []byte = make([]byte, 256)
	for {
		read, err := client.Read(buffer)
		if err != nil {
			Verbose("Connection closed")
			done <- true
			return
		}
		if read > 0 {
			_, err2 := server.Write(buffer[:read])
			if err2 != nil {
				Verbose("Connection closed")
				done <- true
				return
			}
		}
	}
}

func proxyRedis(client net.Conn, name string) {
	defer client.Close()

	Verbose("Proxying :", name)

	for retries := 0; retries < 30; retries++ {
		server, err := net.Dial("unix", *dbs+"/"+name+"/redis.sock")
		if err != nil {
			log.Println("ERROR: connection to "+name+" not successful: ", err, " attempts: ", retries)
		} else {

			done := make(chan bool)

			go glue(client, server, done)
			go glue(server, client, done)

			<-done

			server.Close()
			client.Close()

			<-done

			return
		}
		time.Sleep(1 * time.Second)
	}
	log.Println("FAIL: connection to " + name + " not successful timed out")
}

func sendSimpleReply(conn net.Conn, reply string, inline bool) {
	if inline {
		conn.Write([]byte(reply + "\r\n"))
	} else {
		conn.Write([]byte("*1\r\n$" + strconv.Itoa(len(reply)) + "\r\n" + reply + "\r\n"))
	}
}

func handleConnection(conn net.Conn) {

	var buffer []byte = make([]byte, 256)
	for {
		read, err := conn.Read(buffer)
		if err != nil {
			log.Println("Error handling connection:", err)
			return
		}

		parsed, err := parse(buffer, read)

		if err == nil && parsed.ParamCount == 1 && strings.ToUpper(parsed.Params[0]) == "PING" {
			sendSimpleReply(conn, "-NOAUTH Authentication required.", parsed.Inline)
			continue
		}

		if err == nil && parsed.ParamCount == 2 && strings.ToUpper(parsed.Params[0]) == "AUTH" {
			name := parsed.Params[1]

			redises.RLock()
			ri, running := redises.m[name]
			if ri != nil && !ri.BackupPinged && len(*backupRedis) > 0 {
				go ensureBackupRedis(ri)
			}
			redises.RUnlock()
			if !running {
				startupRedis(parsed.Params[1])
			}
			sendSimpleReply(conn, "OK", parsed.Inline)
			go proxyRedis(conn, parsed.Params[1])
			return
		}

		sendSimpleReply(conn, "-NOAUTH Authentication required.", parsed.Inline)
	}
}

func checkError(err error) {
	if err != nil {
		log.Fatal("Fatal error ", err.Error())
	}
}

func registerExitHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		onExit()
		os.Exit(1)
	}()
}

func onExit() {

	redises.RLock()
	for _, v := range redises.m {
		v.Stop()
	}
	redises.RUnlock()

	if *profile != "" {
		pprof.StopCPUProfile()
	}
	log.Println("Exiting...")
}
