/*

Copyright © 2020 Andrey Kuvshinov. Contacts: <syslinux@protonmail.com>
Copyright © 2020 Eltaline OU. Contacts: <eltaline.ou@gmail.com>
All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

The cTRL project contains unmodified/modified libraries imports too with
separate copyright notices and license terms. Your use of the source code
this libraries is subject to the terms and conditions of licenses these libraries.

*/

package main

import (
	"bytes"
	"crypto/sha512"
	"encoding/gob"
	"fmt"
	"github.com/eltaline/go-waitgroup"
	"github.com/eltaline/nutsdb"
	"github.com/gobuffalo/uuid"
	"github.com/kataras/iris"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Post

// CtrlRun : Start realtime task command or commands method
func CtrlRun(wg *sync.WaitGroup) iris.Handler {
	return func(ctx iris.Context) {
		defer wg.Done()

		fin := make(chan bool)
		kill := make(chan bool)
		wcl := make(chan bool)

		ctx.OnClose(func() {

			select {
			case <-fin:
				return
			default:
				kill <- true
			}

		})

		var err error

		// Wait Group

		wg.Add(1)

		// Loggers

		postLogger, postlogfile := PostLogger()
		defer postlogfile.Close()

		// Shutdown

		if shutdown {

			ctx.StatusCode(iris.StatusInternalServerError)

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		// Variables

		var body []PostTask
		var resp []GetTask
		var p GetTask

		// IP Client

		ip := ctx.RemoteAddr()
		cip := net.ParseIP(ip)
		ush := ctx.GetHeader("Auth")
		vhost := strings.Split(ctx.Host(), ":")[0]

		params := ctx.URLParams()
		length := ctx.GetHeader("Content-Length")

		badhost := true
		baduser := true
		badip := true

		user := ""
		pass := ""
		phsh := ""

		shell := "/bin/bash"

		rthreads := 0

		vtimeout := uint32(28800)

		if ush != "" {

			mchpair := rgxpair.MatchString(ush)

			if !mchpair {

				ctx.StatusCode(iris.StatusBadRequest)

				postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | Bad authorization format", vhost, ip)

				if debugmode {

					_, err = ctx.Writef("[ERRO] Bad authorization format | Virtual Host [%s]\n", vhost)
					if err != nil {
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
					}

				}

				go func() {
					wcl <- true
					fin <- true
				}()

				<-wcl

				return

			}

			sha_512 := sha512.New()

			user = strings.Split(ush, ":")[0]
			pass = strings.Split(ush, ":")[1]

			sha_512.Write([]byte(pass))
			phsh = fmt.Sprintf("%x", sha_512.Sum(nil))

		}

		for _, Server := range config.Server {

			if vhost == Server.HOST {

				badhost = false

				if user != "" && pass != "" {

					for _, Vhost := range ussallow {

						if vhost == Vhost.Vhost {

							for _, PAIR := range Vhost.PAIR {

								if user == PAIR.User && phsh == PAIR.Hash {
									baduser = false
									break
								}
							}

							break

						}

					}

				}

				if baduser {

					for _, Vhost := range ipsallow {

						if vhost == Vhost.Vhost {

							for _, CIDR := range Vhost.CIDR {
								_, ipnet, _ := net.ParseCIDR(CIDR.Addr)
								if ipnet.Contains(cip) {
									badip = false
									break
								}
							}

							break

						}

					}

				}

				shell = Server.SHELL
				rthreads = Server.RTHREADS
				vtimeout = Server.VTIMEOUT

				break

			}

		}

		if badhost {

			ctx.StatusCode(iris.StatusMisdirectedRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 421 | Not found configured virtual host", vhost, ip)

			if debugmode {

				_, err = ctx.Writef("[ERRO] Not found configured virtual host | Virtual Host [%s]\n", vhost)
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		if baduser && badip {

			ctx.StatusCode(iris.StatusForbidden)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 403 | Forbidden", vhost, ip)

			if debugmode {

				_, err = ctx.Writef("[ERRO] Not found allowed user or not found allowed ip | Virtual Host [%s]\n", vhost)
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		if len(params) != 0 {

			ctx.StatusCode(iris.StatusForbidden)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 403 | The query arguments is not allowed during POST request", vhost, ip)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] The query arguments is not allowed during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		clength, err := strconv.ParseInt(length, 10, 64)
		if err != nil {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | Content length error during POST request | Content-Length [%s] | %v", vhost, ip, length, err)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] Content length error during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		if clength == 0 {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was empty during POST request | Content-Length [%s]", vhost, ip, length)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] The body was empty during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		err = ctx.ReadJSON(&body)
		if err != nil {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was bad during POST request | Content-Length [%s]", vhost, ip, length)

			if debugmode {

				//				_, err = ctx.WriteString("[ERRO] The body was bad during POST request\n")
				_, err = ctx.Writef("[ERRO] The body was bad during POST request, %v\n", err)

				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return

		}

		limit := 0

		rc.RLock()
		curthreads := rc.rcounter[vhost]
		rc.RUnlock()

		for {

			if curthreads < rthreads {
				limit = rthreads - curthreads
				break
			}

			select {
			case <-kill:

				go func() {
					wcl <- true
					fin <- true
				}()

				<-wcl

				return

			default:

				time.Sleep(time.Duration(5) * time.Millisecond)

			}

		}

		if limit <= 0 {

			go func() {
				wcl <- true
				fin <- true
			}()

			<-wcl

			return
		}

		qwg := waitgroup.NewWaitGroup(limit)

		for _, task := range body {

			prefskey := task.Key
			preftype := task.Type
			prefpath := task.Path
			preflock := task.Lock
			prefcomm := task.Command
			preftout := task.Timeout

			rc.Lock()
			rc.rcounter[vhost]++
			rc.Unlock()

			qwg.Add(func() {

				defer func() {
					rc.Lock()
					rc.rcounter[vhost]--
					rc.Unlock()
				}()

				skey := prefskey
				ftmst := time.Now().Unix()
				ftype := preftype
				fpath := prefpath
				flock := preflock
				fcomm := prefcomm
				ftout := preftout

				stdcode := 0
				errcode := 0

				stdout := ""
				stderr := ""

				if skey == "" || ftype == "" || fpath == "" || flock == "" || fcomm == "" {

					ctx.StatusCode(iris.StatusBadRequest)

					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was not contains enough parameters during POST request", vhost, ip)

					if debugmode {

						_, err = ctx.WriteString("[ERRO] The body was not contains enough parameters during POST request\n")
						if err != nil {
							postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
						}

					}

					go func() {
						wcl <- true
						fin <- true
					}()

					<-wcl

					return

				}

				if ftout == uint32(0) {
					ftout = vtimeout
				}

				var cmmout bytes.Buffer
				var cmmerr bytes.Buffer

				scm := shell + " -c " + "\"cd " + fpath + " " + "&&" + " " + fcomm + "\""
				cmm := exec.Command(shell, "-c", scm)

				var rtime float64

				stime := time.Now()

				cmm.Stdout = &cmmout
				cmm.Stderr = &cmmerr

				interrupt := false

				cwg := waitgroup.NewWaitGroup(1)

				crun := make(chan bool)

				cwg.Add(func() {

					err = cmm.Start()
					if err != nil {
						interrupt = true
						errcode = 255
						stderr = err.Error()
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | Start command error | Key [%s] | Path [%s] | Lock [%s] | Command [%s] | %v", vhost, ip, skey, fpath, flock, scm, err)
					}

					err = cmm.Wait()
					if err != nil {
						interrupt = true
						errcode = 1
						stderr = err.Error()
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | Execute command error | Key [%s] | Path [%s] | Lock [%s] | Command [%s] | %v", vhost, ip, skey, fpath, flock, scm, err)
					}

					crun <- true

				})

				cmmt := time.After(time.Duration(int(ftout)) * time.Second)

			Kill:

				for {

					select {

					case <-crun:
						break Kill
					case <-kill:
						go func() { kill <- true }()
						_ = cmm.Process.Kill()
						<-crun
						break Kill
					case <-cmmt:
						_ = cmm.Process.Kill()
						<-crun
						errcode = 124
						break Kill

					default:

						time.Sleep(time.Duration(5) * time.Millisecond)

					}

				}

				cwg.Wait()
				close(crun)

				rtime = float64(time.Since(stime)) / float64(time.Millisecond)

				stdout = cmmout.String()

				if !interrupt {
					stderr = cmmerr.String()
				}

				p.Key = skey
				p.Time = ftmst
				p.Type = ftype
				p.Path = fpath
				p.Lock = flock
				p.Command = fcomm
				p.Timeout = ftout
				p.Stdcode = stdcode
				p.Stdout = stdout
				p.Errcode = errcode
				p.Stderr = stderr
				p.Runtime = rtime

				resp = append(resp, p)

			})

		}

		qwg.Wait()

		select {
		case <-kill:
			go func() { fin <- true }()
		default:
			go func() { fin <- true }()
		}

		jkeys, _ := JSONMarshal(resp, true)
		allkeys := string(jkeys)
		rbytes := []byte(allkeys)
		hsize := fmt.Sprintf("%d", len(rbytes))

		ctx.Header("Content-Type", "application/json")
		ctx.Header("Content-Length", hsize)

		_, err = ctx.Write(rbytes)
		if err != nil {
			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
		}

	}

}

// CtrlTask : Start scheduled task command or commands method
func CtrlTask(cldb *nutsdb.DB, wg *sync.WaitGroup) iris.Handler {
	return func(ctx iris.Context) {
		defer wg.Done()

		var err error

		// Wait Group

		wg.Add(1)

		// Loggers

		postLogger, postlogfile := PostLogger()
		defer postlogfile.Close()

		// Shutdown

		if shutdown {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}

		// Variables

		var body []PostTask

		// IP Client

		ip := ctx.RemoteAddr()
		cip := net.ParseIP(ip)
		ush := ctx.GetHeader("Auth")
		vhost := strings.Split(ctx.Host(), ":")[0]

		params := ctx.URLParams()
		length := ctx.GetHeader("Content-Length")

		badhost := true
		baduser := true
		badip := true

		user := ""
		pass := ""
		phsh := ""

		vtimeout := uint32(28800)

		rvbucket := ""

		if ush != "" {

			mchpair := rgxpair.MatchString(ush)

			if !mchpair {

				ctx.StatusCode(iris.StatusBadRequest)

				postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | Bad authorization format", vhost, ip)

				if debugmode {

					_, err = ctx.Writef("[ERRO] Bad authorization format | Virtual Host [%s]\n", vhost)
					if err != nil {
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
					}

				}

				return

			}

			sha_512 := sha512.New()

			user = strings.Split(ush, ":")[0]
			pass = strings.Split(ush, ":")[1]

			sha_512.Write([]byte(pass))
			phsh = fmt.Sprintf("%x", sha_512.Sum(nil))

		}

		for _, Server := range config.Server {

			if vhost == Server.HOST {

				badhost = false

				if user != "" && pass != "" {

					for _, Vhost := range ussallow {

						if vhost == Vhost.Vhost {

							for _, PAIR := range Vhost.PAIR {

								if user == PAIR.User && phsh == PAIR.Hash {
									baduser = false
									break
								}
							}

							break

						}

					}

				}

				if baduser {

					for _, Vhost := range ipsallow {

						if vhost == Vhost.Vhost {

							for _, CIDR := range Vhost.CIDR {
								_, ipnet, _ := net.ParseCIDR(CIDR.Addr)
								if ipnet.Contains(cip) {
									badip = false
									break
								}
							}

							break

						}

					}

				}

				vtimeout = Server.VTIMEOUT
				rvbucket = "recv" + "_" + vhost + ":"

				break

			}

		}

		if badhost {

			ctx.StatusCode(iris.StatusMisdirectedRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 421 | Not found configured virtual host", vhost, ip)

			if debugmode {

				_, err = ctx.Writef("[ERRO] Not found configured virtual host | Virtual Host [%s]\n", vhost)
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		if baduser && badip {

			ctx.StatusCode(iris.StatusForbidden)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 403 | Forbidden", vhost, ip)

			if debugmode {

				_, err = ctx.Writef("[ERRO] Not found allowed user or not found allowed ip | Virtual Host [%s]\n", vhost)
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		if len(params) != 0 {

			ctx.StatusCode(iris.StatusForbidden)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 403 | The query arguments is not allowed during POST request", vhost, ip)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] The query arguments is not allowed during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		clength, err := strconv.ParseInt(length, 10, 64)
		if err != nil {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | Content length error during POST request | Content-Length [%s] | %v", vhost, ip, length, err)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] Content length error during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		if clength == 0 {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was empty during POST request | Content-Length [%s]", vhost, ip, length)

			if debugmode {

				_, err = ctx.WriteString("[ERRO] The body was empty during POST request\n")
				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		err = ctx.ReadJSON(&body)
		if err != nil {

			ctx.StatusCode(iris.StatusBadRequest)

			postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was bad during POST request | Content-Length [%s]", vhost, ip, length)

			if debugmode {

				//				_, err = ctx.WriteString("[ERRO] The body was bad during POST request\n")
				_, err = ctx.Writef("[ERRO] The body was bad during POST request, %v\n", err)

				if err != nil {
					postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
				}

			}

			return

		}

		for _, task := range body {

			stdcode := 0
			errcode := 0

			stdout := ""
			stderr := ""

			if task.Key == "" || task.Type == "" || task.Path == "" || task.Lock == "" || task.Command == "" {

				ctx.StatusCode(iris.StatusBadRequest)

				postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 400 | The body was not contains enough parameters during POST request", vhost, ip)

				if debugmode {

					_, err = ctx.WriteString("[ERRO] The body was not contains enough parameters during POST request\n")
					if err != nil {
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
					}

				}

				return

			}

			pbuffer := new(bytes.Buffer)
			enc := gob.NewEncoder(pbuffer)

			internal := uuid.Must(uuid.NewV4())

			ftmst := time.Now().Unix()

			bkey := []byte("t:" + fmt.Sprintf("%d", ftmst) + ":" + fmt.Sprintf("%x", internal) + ":" + task.Key)
			skey := task.Key

			ftype := task.Type
			fpath := task.Path
			flock := task.Lock
			fcomm := task.Command

			ftout := vtimeout

			if task.Timeout > uint32(0) {
				ftout = task.Timeout
			}

			etsk := &RawTask{
				Time:    ftmst,
				Type:    ftype,
				Path:    fpath,
				Lock:    flock,
				Command: fcomm,
				Timeout: ftout,
				Stdcode: stdcode,
				Stdout:  stdout,
				Errcode: errcode,
				Stderr:  stderr,
				Runtime: float64(0),
			}

			err = enc.Encode(etsk)
			if err != nil {

				ctx.StatusCode(iris.StatusInternalServerError)

				postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 500 | Gob task encode error | Key [%s] | Path [%s] | Lock [%s] | Command [%s] | %v", vhost, ip, skey, fpath, flock, fcomm, err)

				if debugmode {

					_, err = ctx.WriteString("[ERRO] Gob task encode error\n")
					if err != nil {
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
					}

				}

				return

			}

			err = NDBInsert(cldb, rvbucket, bkey, pbuffer.Bytes(), 0)
			if err != nil {

				ctx.StatusCode(iris.StatusInternalServerError)

				postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 500 | Insert received task db error | Key [%s] | Path [%s] | Lock [%s] | Command [%s] | %v", vhost, ip, skey, fpath, flock, fcomm, err)

				if debugmode {

					_, err = ctx.WriteString("[ERRO] Insert received task db error\n")
					if err != nil {
						postLogger.Errorf("| Virtual Host [%s] | Client IP [%s] | 499 | Can`t complete response to client | %v", vhost, ip, err)
					}

				}

				return

			}

		}

	}

}
