package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var database *sql.DB

type DataHTML struct {
	Data []ReustarantHTML
}

type ReustarantHTML struct {
	Id                     int
	Name                   string
	PersonsCountAvailibale int
}

type Reustarant struct {
	Name   string
	Tables map[int]int //sits:countTables
}

func getDBKey() string{
	file, err := os.Open("dbKey.txt")
	if err != nil{
        fmt.Println(err) 
        os.Exit(1) 
    }
    defer file.Close()
	
	data := make([]byte, 64)
	strKey := ""

	for{
        n, err := file.Read(data)
        if err == io.EOF{   // если конец файла
            break           // выходим из цикла
        }
        fmt.Print(string(data[:n]))
		strKey += string(data[:n])
    }
	return strKey
}

func isContainsName(m map[int]Reustarant, name string) (bool, int) {
	var contains bool = false
	var idRet int = -1
	for id, rest := range m {
		if rest.Name == name {
			contains = true
			idRet = id
			break
		}
	}
	return contains, idRet
}

func deepCopy(m map[int]Reustarant) map[int]Reustarant {
	var newMap map[int]Reustarant = map[int]Reustarant{}
	for id, reustarant := range m {
		var newTables map[int]int = map[int]int{}
		for tables, count := range reustarant.Tables {
			newTables[tables] = count
		}
		newRest := Reustarant{
			Name:   reustarant.Name,
			Tables: newTables,
		}
		newMap[id] = newRest
	}
	return newMap
}

//получить стол с максимальным количеством мест
func (r Reustarant) getMaxSits() int {
	maxSits := 0
	for tableSits, _ := range r.Tables {
		if tableSits > maxSits {
			maxSits = tableSits
		}
	}
	return maxSits
}

//получить стол с минимальым количеством мест
func (r Reustarant) getMinSits() int {
	var minSits int
	var p int = 0
	for sits, _ := range r.Tables {
		if p < 1 {
			minSits = sits
			p++
		} else {
			if sits < minSits {
				minSits = sits
			}
		}
	}
	return minSits
}

//получить общее количество доступных мест
func (r Reustarant) GetSitsCount() int {
	var count int = 0
	for sits, countSits := range r.Tables {
		count += sits * countSits
	}
	return count
}

//создает хэш с ресторанами и столами по запросу
func createMapRestuarantTables(rows *sql.Rows, err error) map[int]Reustarant {
	var mapReustarant map[int]Reustarant = map[int]Reustarant{}
	if err == nil {
		for rows.Next() {
			var idRest int
			var nameRest string
			var tableSeats int
			var tableCount int
			rows.Scan(&idRest, &nameRest, &tableSeats, &tableCount)

			_, existed := mapReustarant[idRest]

			if !existed {
				newRest := Reustarant{
					Name:   nameRest,
					Tables: map[int]int{},
				}
				mapReustarant[idRest] = newRest

			}

			mapReustarant[idRest].Tables[tableSeats] += tableCount
		}
	} else {
		fmt.Println(err)
	}
	return mapReustarant
}

//создает хэш со всеми ресторанами и всеми столами в распоряжении
func createFullTablesList() map[int]Reustarant {
	var listReustarant map[int]Reustarant = map[int]Reustarant{}
	var query string = "select r.id,  r.\"name\", t.seats, t.\"count\" " +
		"from \"tables\"  t " +
		"join reustarants r on t.id_reustarant = r.id " +
		"order by r.\"time\", r.price"
	rows, err := database.Query(query)
	listReustarant = createMapRestuarantTables(rows, err)
	return listReustarant
}

//создает хэш с ресторанами со всеми зарезервированными столиками на определенное время
func getReservetListInTime(hour int, minute int) map[int]Reustarant {
	var reservedTables map[int]Reustarant = map[int]Reustarant{}
	var timeD string = strconv.Itoa(hour) + ":" + strconv.Itoa(minute)
	var query string = "select rest.id,  rest.\"name\", t.seats, reserv.reservation_count " +
		"from \"tables\"  t " +
		"join reustarants rest on t.id_reustarant = rest.id " +
		"join reservations reserv  on reserv.id_table = t.id " +
		"where reserv.\"time\" > time '" + timeD + "'-interval '2 hour' and reserv.\"time\" < time '" + timeD + "'+interval '2 hour' "
	rows, err := database.Query(query)
	reservedTables = createMapRestuarantTables(rows, err)

	return reservedTables
}

//создает хэш со свободными столами путем вычитания из всех столов занятые
func getAvailibaleTables(fullTables map[int]Reustarant, reservedTables map[int]Reustarant) map[int]Reustarant {
	availableTables := fullTables
	for idRest, _ := range reservedTables {
		for tableSeats, _ := range reservedTables[idRest].Tables {
			tmp := availableTables[idRest].Tables[tableSeats]
			tmp = tmp - reservedTables[idRest].Tables[tableSeats]
			availableTables[idRest].Tables[tableSeats] = tmp

			if availableTables[idRest].Tables[tableSeats] == 0 {
				delete(availableTables[idRest].Tables, tableSeats)
			}
		}
	}

	return availableTables
}

//получить список всех ресторанов доступных для размещение n человек. здесь список столов это предпалагаемый набор для бронирования
func getAvailibaleReustarant(availibleTables map[int]Reustarant, peopleCount int) map[int]Reustarant { 
	var reustarants map[int]Reustarant = map[int]Reustarant{} 
	for idRest, rest := range availibleTables {
		var count int = peopleCount
		var tables map[int]int = map[int]int{}
		for len(rest.Tables) > 0 && count > 0 {
			tableSits := rest.getMaxSits()
			if count < tableSits {
				tableSits = rest.getMinSits()
			}
			tables[tableSits] += 1
			count -= tableSits
			rest.Tables[tableSits] -= 1

			if rest.Tables[tableSits] < 1 {
				delete(rest.Tables, tableSits)
			}
		}
		if count < 1 {
			tmpRest := Reustarant{
				Name:   rest.Name,
				Tables: tables,
			}
			reustarants[idRest] = tmpRest
		}
	}

	return reustarants
}

//создает сортированный список ресторанчиков (потому что хэши не сортируемы) и для html документа
func getSortedAvailibaleReustarants(availbleReustarants map[int]Reustarant, forCount map[int]Reustarant) []ReustarantHTML {
	var restList []ReustarantHTML = []ReustarantHTML{}
	var query string = "select r.\"name\" " +
		"from reustarants r " +
		"order by r.\"time\", r.price "
	rows, _ := database.Query(query)

	for rows.Next() {
		var name string
		rows.Scan(&name)
		isContains, id := isContainsName(availbleReustarants, name)
		if isContains {
			restHtml := ReustarantHTML{
				Id:                     id,
				Name:                   name,
				PersonsCountAvailibale: forCount[id].GetSitsCount(),
			}
			restList = append(restList, restHtml)
		}

	}

	return restList
}

//создает нового человека в базе данных
func putNewPerson(name string, phone string, count int) int {
	var id int
	rows, _ := database.Query("select max(person.id) from person")
	for rows.Next() {
		rows.Scan(&id)
	}
	id++

	result, err := database.Exec("insert into person (id,\"name\",phone,\"count\") values ($1,$2,$3,$4)", id, name, phone, count)
	fmt.Println(result, err)
	return id
}

//получить id стола для бд
func getIdTable(idRest int, sits int) int {
	var query string = "select t.id " +
		"from \"tables\" t " +
		"where t.id_reustarant =" + strconv.Itoa(idRest) + " and t.seats = " + strconv.Itoa(sits)
	var idTable int
	rows, _ := database.Query(query)

	for rows.Next() {
		rows.Scan(&idTable)
	}

	return idTable
}

//создает новую запись в бд с зарезервированными столиками
func putNewReservation(reustarant Reustarant, idRest int, hour int, minute int, name string, phone string, count int) {
	idPerson := putNewPerson(name, phone, count)
	time := strconv.Itoa(hour) + ":" + strconv.Itoa(minute)
	for sits, countTable := range reustarant.Tables {
		idTable := getIdTable(idRest, sits)
		result, err := database.Exec("insert into reservations (id,id_person,id_table,reservation_count,\"time\") values ((select max(r.id) from reservations r)+1 ,$1,$2,$3,$4)",
			idPerson, idTable, countTable, time)
		fmt.Println("бронирование", result, err)
	}
}


func showIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			log.Println(err)
		}
		var peopleCount string = r.FormValue("peopleCount")
		hour, _ := strconv.Atoi(r.FormValue("hour"))
		minute, _ := strconv.Atoi(r.FormValue("minute"))
		if !(hour == 21 && minute > 0) {
			nextPageLink := "/" + strconv.Itoa(hour) + "/" + strconv.Itoa(minute) + "/" + peopleCount
			http.Redirect(w, r, nextPageLink, 301)
		}

	}

	http.ServeFile(w, r, "templates/index.html")
}

func showVariants(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hour, _ := strconv.Atoi(vars["hour"])
	minute, _ := strconv.Atoi(vars["minute"])
	count, _ := strconv.Atoi(vars["count"])
	if (hour < 9 || hour > 21) || minute > 59 || count < 1 {
		http.Redirect(w, r, "/error", 301)
	}

	allTables := createFullTablesList()
	reservedTables := getReservetListInTime(hour, minute)
	availibaleTables := getAvailibaleTables(allTables, reservedTables)
	avTforCount := deepCopy(availibaleTables) //никто не предупреждал что хэш это ссылка хотя наверно можно было догадаться
	availibaleReustarants := getAvailibaleReustarant(availibaleTables, count)

	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			log.Println(err)
		}
		idReustarant, _ := strconv.Atoi(r.FormValue("reustarant"))
		name := r.FormValue("name")
		phone := r.FormValue("phone")

		if name != "" && phone != "" {
			reustarant := availibaleReustarants[idReustarant]
			putNewReservation(reustarant, idReustarant, hour, minute, name, phone, count)
			http.Redirect(w, r, "/complite", 301)
		}
	}

	sortedReustarantHtml := getSortedAvailibaleReustarants(availibaleReustarants, avTforCount)
	data := DataHTML{
		Data: sortedReustarantHtml,
	}

	tmpl, _ := template.ParseFiles("templates/second.html")
	tmpl.Execute(w, data)
}

func showError(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "templates/err.html")
}

func showCompl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		http.Redirect(w, r, "/", 301)
	}
	http.ServeFile(w, r, "templates/complite.html")
}

func main() {
	var dbKey string = getDBKey()
	connStr := dbKey //"user=postgres password=admin dbname=postgres sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Println(err)
	}
	database = db
	defer db.Close()

	router := mux.NewRouter()

	router.HandleFunc("/", showIndex)
	router.HandleFunc("/{hour:[0-9]+}/{minute:[0-9]+}/{count:[0-9]+}", showVariants)
	router.HandleFunc("/error", showError)
	router.HandleFunc("/complite", showCompl)

	http.Handle("/", router)
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./css"))))

	fmt.Println("Server is listening...")
	http.ListenAndServe(":8181", nil)

}
