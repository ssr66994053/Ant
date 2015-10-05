# ant 
ant is a file move tool writen by Go.

## Install
    go get -u github.com/ssr66994053/ant
    go build
    go install

## Usage:
    ant [flags] FROM PATTERN TO [FROM PATTERN TO]

* ###flags:
    -h                  print usage
    -v                  print version number
    -loop=n, -loop n    loop n times, sleep 5 second per loop. 
                        default n = 0
    -time=m, -time m    run per time duration. like '5s' '1h'.
                        default m = 0s
    -cron=c, -cron c    run with crontab. like "*/5 * * * * ?".
                        default c = "none"
priority: -cron > -time > -loop > default mode[-loop=1]

* ###FROM:
ant can watch the [**FROM**] directory(if the dir is exists) if there is a file name match the [**PATTERN**] then move the file to the [**TO**] directory. keyword[**$**] present last [**FROM**]. 

* ###PATTERN:
it must be a regexp. like "H.* ", so "Hello World!" will be match. keyword[**$**] present last [**PATTERN**].

* ###TO:
if file is matched, then it will move to the [**TO**] directory(if it is not exists, ant will create it, otherwise ant will rewrite it). keyword[**$**] present last [**TO**].

* ###Tips:
some special keywords in [FROM][PATTERN][TO] can be replace when ant run.
 
* {date}      date.                 like 20060102
* {time}      time.                 like 150405
* {year}      year.                 like 2005
* {month}     month.                like 01
* {day}       day.                  like 02
* {hour}      hour.                 like 15
* {minute}    minute.               like 04
* {second}    second.               like 05
* {yesterday} yesterday.            like 20060102
* {day-1}     yesterday.            like 20060102
* {day-2} the day before yesterday. like 20060101

##Example:
    ant -loop=10 d:/from t[\\d]+.txt d:/to
    ant -time=5s d:/from1 t[\\d]+.txt d:/to1 d:/from2 t[a-z]+.txt d:/to2
    ant -cron="*/5 * * * * ?" d:/from t[\\d]+.txt d:/to1 $ t[a-z]+.txt d:/to2
    ant d:/from/{date} t[\\d]+.txt d:/to/{date}
    ant d:/from/ t[\\d]+.txt d:/to1 $ d:/to2
    ant d:/from/ t[\\d]+.txt d:/to t[a-z]+.txt $
	