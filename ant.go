package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron"
)

const usageInfo = `ant is a file move tool writen by Go.

Usage:
    ant [flags] FROM PATTERN TO [FROM PATTERN TO]

flags:
    -h                  print usage
    -v                  print version number

    -loop=n, -loop n    loop n times, sleep 5 second per loop. 
                        default n = 0
    -time=m, -time m    run per time duration. like '5s' '1h'.
                        default m = 0s
    -cron=c, -cron c    run with crontab. like "*/5 * * * * ?".
                        default c = "none"
    priority: -cron > -time > -loop > default mode[-loop=1]

FROM:
    ant can watch the [FROM] directory(if the dir is exists) if there 
    is a file name match the [PATTERN] then move the file to the [TO] 
    directory. keyword[$] present last [FROM]. 

PATTERN:
    it must be a regexp. like "H.* ", so "Hello World!" will be match.
    keyword[$] present last [PATTERN].

TO:
    if file is matched, then it will move to the [TO] directory(if it 
    is not exists, ant will create it, otherwise ant will rewrite it). 
    keyword[$] present last [TO].

Tips:
    some special keywords in [FROM][PATTERN][TO] can be replace when 
    ant run.
    keyword: 
        {date}      date.                 like 20060102
        {time}      time.                 like 150405
        {year}      year.                 like 2005
        {month}     month.                like 01
        {day}       day.                  like 02
        {hour}      hour.                 like 15
        {minute}    minute.               like 04
        {second}    second.               like 05
        {yesterday} yesterday.            like 20060102
        {day-1}     yesterday.            like 20060102
        {day-2} the day before yesterday. like 20060101

Example:
    ant -loop=10 d:/from t[\\d]+.txt d:/to
    ant -time=5s d:/from1 t[\\d]+.txt d:/to1 d:/from2 t[a-z]+.txt d:/to2
    ant -cron="*/5 * * * * ?" d:/from t[\\d]+.txt d:/to1 $ t[a-z]+.txt d:/to2
    ant d:/from/{date} t[\\d]+.txt d:/to/{date}
    ant d:/from/ t[\\d]+.txt d:/to1 $ d:/to2
    ant d:/from/ t[\\d]+.txt d:/to t[a-z]+.txt $
`

const (
	version            = "0.1" // 版本号
	sleep_time         = 5 * time.Second
	default_loop       = 0
	default_time       = 0 * time.Second
	default_cron       = "none"
	time_format_date   = "20060102"
	time_format_time   = "150405"
	time_format_year   = "2006"
	time_format_month  = "01"
	time_format_day    = "02"
	time_format_hour   = "15"
	time_format_minute = "04"
	time_format_second = "05"
)

const (
	run_mode_default = 1 << iota
	run_mode_loop
	run_mode_time
	run_mode_cron
)

var (
	hlp   bool
	ver   bool
	loop  int
	timeD time.Duration
	cronT string
)

type moveJob struct {
	wMap map[string]map[string][]string // map[key:from]->map[key:to]->[]reg
}

func (this *moveJob) Run() {
	log.Println("Job is running...")
	printWmap(this.wMap)
	t := time.Now()
	wait := &sync.WaitGroup{}
	for k, v := range this.wMap {
		wait.Add(1)
		go func(path string, toMap map[string][]string) {
			defer wait.Done()
			path = replaceKeywords(path, t)
			fi, err := os.Stat(path)
			if os.IsNotExist(err) {
				fmt.Printf("path:[%s] is not exist.\n", path)
				return
			}
			if fi.IsDir() {
				walkWait := &sync.WaitGroup{}
				filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
					if path == "" {
						fmt.Println("path string is empty.")
						return nil
					}
					if info == nil {
						fmt.Println("info os.FileInfo is nil.")
						return nil
					}
					if info.IsDir() {
						fmt.Printf("path %s is dir.\n", path)
						return nil
					}
					walkWait.Add(1)
					go func() {
						defer walkWait.Done()
						content, err := readFile(path)
						if err != nil {
							fmt.Println(err)
							return
						}
						matchWait := &sync.WaitGroup{}
						flag := false
						for to, ps := range toMap {
							matchWait.Add(1)
							go func(to string, ps []string) {
								defer matchWait.Done()
								toPath := replaceKeywords(to, t)
								err = makeDir(toPath)
								if err != nil {
									fmt.Println(err)
									return
								}
								toFull := toPath + "/" + info.Name()
								patterns := []string{}
								for _, p := range ps {
									patterns = append(patterns, replaceKeywords(p, t))
								}
								res, err := writeFileIfMatch(toFull, info.Name(), content, patterns)
								flag = flag || res
								if err != nil {
									fmt.Println(err)
									return
								}
							}(to, ps)
						}
						matchWait.Wait()
						if flag {
							err = os.Remove(path)
							if err != nil {
								fmt.Println(err)
								fmt.Printf("remove %s error.\n", path)
							}
						}
					}()
					return nil
				})
				walkWait.Wait()
			} else {
				fmt.Printf("path:[%s] is not a dir.\n", path)
			}
		}(k, v)
	}
	wait.Wait()
	log.Println("Job is done...")
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("open %d error.", path)
	}
	content, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("read %d error.", path)
	}
	return content, nil
}

func makeDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("to path:[%s] is not exist and ant will create it.\n", path)
		if err := os.MkdirAll(path, os.ModeDir); err != nil {
			fmt.Println(err)
			return fmt.Errorf("make dir %s error.", path)
		}
	}
	return nil
}

func writeFileIfMatch(path string, filename string, content []byte, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		reg, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Println(err)
			return false, fmt.Errorf("%s compile err.", pattern)
		}
		if reg.Match([]byte(filename)) {
			err := ioutil.WriteFile(path, content, os.ModePerm)
			if err != nil {
				fmt.Println(err)
				return false, fmt.Errorf("write %s error.\n", path)
			}
			return true, nil
		}
	}
	return false, nil
}

func printWmap(wMap map[string]map[string][]string) {
	for from, toMap := range wMap {
		fmt.Println("from :", from)
		for to, ps := range toMap {
			fmt.Printf("\tto : %s\n", to)
			for _, p := range ps {
				fmt.Printf("\t\t%s\n", p)
			}
		}
	}
}

func init() {
	flag.BoolVar(&hlp, "h", false, "Print Usage")
	flag.BoolVar(&ver, "v", false, "Print Version Number")
	flag.IntVar(&loop, "loop", default_loop, "Loop Times")
	flag.DurationVar(&timeD, "time", default_time, "Run Every Duration Time")
	flag.StringVar(&cronT, "cron", default_cron, "Run With Crontab")
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	if ver {
		fmt.Println("Version:", version)
		os.Exit(0)
	}
	if hlp {
		usage()
	}

	runMode := run_mode_default
	if cronT != default_cron {
		runMode = run_mode_cron
	} else if timeD != default_time {
		runMode = run_mode_time
	} else if loop != default_loop {
		runMode = run_mode_loop
	} else {
		fmt.Println("do not set -loop, -time or -cron. ant wiil run with default mode(loop once).")
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGKILL)
	defer close(sigCh)
	c := cron.New()

	w, err := filter(args)
	if err != nil {
		fmt.Println(err)
		os.Exit(0)
	}

	if runMode == run_mode_cron {
		// crontab
		schedule, err := cron.Parse(cronT)
		if err != nil {
			fmt.Println(err)
			os.Exit(0)
		}

		c.Schedule(schedule, &moveJob{w})
		c.Start()
		defer c.Stop()
	} else if runMode == run_mode_time {
		// time
		schedule := cron.Every(timeD)
		c := cron.New()
		c.Schedule(schedule, &moveJob{w})
		c.Start()
		defer c.Stop()
	} else if runMode == run_mode_loop {
		// loop
		job := &moveJob{w}
		for i := 0; i < loop; i++ {
			job.Run()
			time.Sleep(5 * time.Second)
		}
		os.Exit(0)
	} else if runMode == run_mode_default {
		// default
		job := &moveJob{w}
		job.Run()
		os.Exit(0)
	} else {
		fmt.Printf("run mode [%d] unknown.\n", runMode)
		os.Exit(0)
	}

	sig := <-sigCh
	fmt.Println("Got signal:", sig)
}

func usage() {
	fmt.Println(usageInfo)
	os.Exit(0)
}

func filter(args []string) (map[string]map[string][]string, error) {
	t := time.Now()
	wMap := make(map[string]map[string][]string)

	if len(args) >= 3 {
		if len(args)%3 == 0 {
			lastFrom := args[0]
			lastPattern := args[1]
			lastTo := args[2]
			if lastFrom == "" {
				return nil, fmt.Errorf("first [FROM] is empty.")
			}
			if lastPattern == "" {
				return nil, fmt.Errorf("first [PATTERN] is empty.")
			}
			if lastTo == "" {
				return nil, fmt.Errorf("first [TO] is empty.")
			}
			for i := 0; i < len(args); i = i + 3 {
				from := args[i]
				pat := args[i+1]
				to := args[i+2]
				if from == "$" {
					from = lastFrom
				} else if from != lastFrom {
					lastFrom = from
				}
				if pat == "$" {
					pat = lastPattern
				} else if pat != lastPattern {
					lastPattern = pat
				}
				if to == "$" {
					to = lastTo
				} else if to != lastTo {
					lastTo = to
				}
				if toMap, ok := wMap[from]; ok {
					if pArr, ok := toMap[to]; ok {
						flag := false
						for _, v := range pArr {
							if v == pat {
								flag = true
								break
							}
						}
						if !flag {
							rpat := replaceKeywords(pat, t)
							_, err := regexp.Compile(rpat)
							if err != nil {
								fmt.Println(err)
								fmt.Printf("%s(after replace %s) compile err.", pat, rpat)
								os.Exit(0)
							}
							pArr = append(pArr, pat)
							toMap[to] = pArr
						}
					} else {
						rpat := replaceKeywords(pat, t)
						_, err := regexp.Compile(rpat)
						if err != nil {
							fmt.Println(err)
							fmt.Printf("%s(after replace %s) compile err.", pat, rpat)
							os.Exit(0)
						}
						toMap[to] = []string{pat}
					}
				} else {
					rpat := replaceKeywords(pat, t)
					_, err := regexp.Compile(rpat)
					if err != nil {
						fmt.Println(err)
						fmt.Printf("%s(after replace %s) compile err.", pat, rpat)
						os.Exit(0)
					}
					tmp := make(map[string][]string)
					tmp[to] = []string{pat}
					wMap[from] = tmp
				}
			}
			if len(wMap) == 0 {
				return nil, fmt.Errorf("No validate [FROM PATTERN TO].")
			}
			return wMap, nil
		} else {
			return nil, fmt.Errorf("length of args is %d, length-1 must be %3 == 0.", len(args)+1)
		}
	} else {
		return nil, fmt.Errorf("length of args is %d, length must be >= 3.", len(args))
	}
}

func replaceKeywords(src string, t time.Time) string {
	src = strings.Replace(src, "{date}", getReplace(t, time_format_date), -1)
	src = strings.Replace(src, "{time}", getReplace(t, time_format_time), -1)
	src = strings.Replace(src, "{year}", getReplace(t, time_format_year), -1)
	src = strings.Replace(src, "{month}", getReplace(t, time_format_month), -1)
	src = strings.Replace(src, "{day}", getReplace(t, time_format_day), -1)
	src = strings.Replace(src, "{hour}", getReplace(t, time_format_hour), -1)
	src = strings.Replace(src, "{minute}", getReplace(t, time_format_minute), -1)
	src = strings.Replace(src, "{second}", getReplace(t, time_format_second), -1)
	t = t.Add(-24 * time.Hour)
	src = strings.Replace(src, "{yesterday}", getReplace(t, time_format_second), -1)
	src = strings.Replace(src, "{day-1}", getReplace(t, time_format_second), -1)
	t = t.Add(-24 * time.Hour)
	src = strings.Replace(src, "{day-2}", getReplace(t, time_format_second), -1)
	return src
}

func getReplace(t time.Time, fotmat string) string {
	return t.Format(fotmat)
}
