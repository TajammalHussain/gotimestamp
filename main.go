package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
}).ParseGlob("templates/*.html"))

type User struct {
	ID       int
	Username string
	Password string
	Role     string
}

type SessionUser struct {
	Username string
	Role     string
}

type ShiftRecord struct {
	Employee   string
	CheckIn    string
	CheckOut   string
	Duration   string
	AddressIn  string
	AddressOut string
}

func initDB() {
	var err error
	os.MkdirAll("./data", os.ModePerm)
	db, err = sql.Open("sqlite", "./data/database.db")
	if err != nil {
		panic(err)
	}
	if err = db.Ping(); err != nil {
		panic(err)
	}
	createTables()
	seedSuperAdmin()
}

func createTables() {
	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE,
		password TEXT,
		role TEXT
	);`)
	db.Exec(`CREATE TABLE IF NOT EXISTS attendance (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		employee TEXT,
		check_in TEXT,
		check_out TEXT,
		duration TEXT,
		address_in TEXT,
		address_out TEXT
	);`)
}

func seedSuperAdmin() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE username='superadmin'").Scan(&count)
	if count == 0 {
		db.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, ?)", "superadmin", "super123", "superadmin")
	}
	// Keep legacy admin if exists
	db.QueryRow("SELECT COUNT(*) FROM users WHERE username='admin'").Scan(&count)
	if count == 0 {
		db.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, ?)", "admin", "admin123", "admin")
	}
}

func reverseGeocode(lat, lon string) string {
	if lat == "" || lon == "" {
		return "Unknown location"
	}
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%s&lon=%s&format=json", lat, lon)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "GoApp")
	resp, err := client.Do(req)
	if err != nil {
		return "Unknown location"
	}
	defer resp.Body.Close()
	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	if name, ok := data["display_name"].(string); ok {
		return name
	}
	return "Unknown location"
}

func validateUser(username, password string) string {
	var role string
	err := db.QueryRow("SELECT role FROM users WHERE username=? AND password=?", username, password).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func getSessionUser(r *http.Request) *SessionUser {
	cookie, err := r.Cookie("user")
	if err != nil {
		return nil
	}
	parts := strings.Split(cookie.Value, "|")
	if len(parts) != 2 {
		return nil
	}
	return &SessionUser{Username: parts[0], Role: parts[1]}
}

func setSessionUser(w http.ResponseWriter, username, role string) {
	http.SetCookie(w, &http.Cookie{Name: "user", Value: username + "|" + role, Path: "/"})
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "user", Value: "", Path: "/", MaxAge: -1})
}

func getLastOpenCheckIn(username string) (string, string) {
	var checkIn, address string
	db.QueryRow(`SELECT check_in, address_in FROM attendance WHERE employee=? AND check_out='' ORDER BY check_in DESC LIMIT 1`, username).Scan(&checkIn, &address)
	return checkIn, address
}

func isCheckedIn(username string) bool {
	var checkOut string
	err := db.QueryRow(`SELECT check_out FROM attendance WHERE employee=? ORDER BY check_in DESC LIMIT 1`, username).Scan(&checkOut)
	if err != nil {
		return false
	}
	return checkOut == ""
}

func appendAttendance(employee, checkIn, checkOut, duration, addressIn, addressOut string) {
	db.Exec(`INSERT INTO attendance (employee, check_in, check_out, duration, address_in, address_out) VALUES (?, ?, ?, ?, ?, ?)`,
		employee, checkIn, checkOut, duration, addressIn, addressOut)
}

/* ── PERMISSION HELPERS ─────────────────────────────────────────────────── */

func isSuperAdmin(u *SessionUser) bool { return u != nil && u.Role == "superadmin" }
func isAdmin(u *SessionUser) bool      { return u != nil && (u.Role == "admin" || u.Role == "superadmin") }

/* ── HANDLERS ────────────────────────────────────────────────────────────── */

func rootHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if u.Role == "employee" {
		http.Redirect(w, r, "/employee", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "login.html", nil)
		return
	}
	role := validateUser(r.FormValue("username"), r.FormValue("password"))
	if role == "" {
		templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Invalid login"})
		return
	}
	setSessionUser(w, r.FormValue("username"), role)
	if role == "employee" {
		http.Redirect(w, r, "/employee", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func employeeHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if u == nil || u.Role != "employee" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	checkedIn := isCheckedIn(u.Username)
	lastCheckIn, _ := getLastOpenCheckIn(u.Username)
	var finalDuration string
	if !checkedIn {
		db.QueryRow(`SELECT duration FROM attendance WHERE employee=? AND check_out != '' ORDER BY check_in DESC LIMIT 1`, u.Username).Scan(&finalDuration)
	}
	templates.ExecuteTemplate(w, "employee.html", map[string]interface{}{
		"Username":      u.Username,
		"CheckedIn":     checkedIn,
		"LastCheckIn":   lastCheckIn,
		"FinalDuration": finalDuration,
	})
}

func submitAttendanceHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	status := r.FormValue("status")
	lat := r.FormValue("latitude")
	lon := r.FormValue("longitude")
	now := time.Now().Format(time.RFC3339)
	address := reverseGeocode(lat, lon)

	if status == "Check-In" {
		appendAttendance(u.Username, now, "", "", address, "")
	}
	if status == "Check-Out" {
		lastIn, _ := getLastOpenCheckIn(u.Username)
		if lastIn == "" {
			http.Redirect(w, r, "/employee", http.StatusSeeOther)
			return
		}
		t1, _ := time.Parse(time.RFC3339, lastIn)
		t2, _ := time.Parse(time.RFC3339, now)
		diff := t2.Sub(t1)
		duration := fmt.Sprintf("%02d:%02d:%02d", int(diff.Hours()), int(diff.Minutes())%60, int(diff.Seconds())%60)
		var openID int
		db.QueryRow(`SELECT id FROM attendance WHERE employee=? AND check_out='' ORDER BY check_in DESC LIMIT 1`, u.Username).Scan(&openID)
		if openID > 0 {
			db.Exec(`UPDATE attendance SET check_out=?, duration=?, address_out=? WHERE id=?`, now, duration, address, openID)
		}
	}
	http.Redirect(w, r, "/employee", http.StatusSeeOther)
}

/* ── ADMIN HANDLERS ─────────────────────────────────────────────────────── */

func adminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isAdmin(u) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	rows, err := db.Query("SELECT id, username, password, role FROM users ORDER BY role, username")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var usr User
		rows.Scan(&usr.ID, &usr.Username, &usr.Password, &usr.Role)
		users = append(users, usr)
	}
	templates.ExecuteTemplate(w, "admin_dashboard.html", map[string]interface{}{
		"Username":     u.Username,
		"Role":         u.Role,
		"IsSuperAdmin": isSuperAdmin(u),
		"Users":        users,
	})
}

func adminLogsHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isAdmin(u) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// Date filter params
	filter := r.URL.Query().Get("filter")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	now := time.Now()
	switch filter {
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		from = now.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")
		to = now.Format("2006-01-02")
	case "monthly":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		to = now.Format("2006-01-02")
	}

	query := `SELECT employee, check_in, check_out, duration, address_in, address_out FROM attendance`
	args := []interface{}{}
	if from != "" && to != "" {
		query += ` WHERE date(check_in) >= ? AND date(check_in) <= ?`
		args = append(args, from, to)
	}
	query += ` ORDER BY check_in DESC`

	rows, _ := db.Query(query, args...)
	defer rows.Close()

	logs := make(map[string][]ShiftRecord)
	var allRecords []ShiftRecord
	for rows.Next() {
		var rec ShiftRecord
		rows.Scan(&rec.Employee, &rec.CheckIn, &rec.CheckOut, &rec.Duration, &rec.AddressIn, &rec.AddressOut)
		if len(rec.CheckIn) < 10 {
			continue
		}
		date := rec.CheckIn[:10]
		logs[date] = append(logs[date], rec)
		allRecords = append(allRecords, rec)
	}

	templates.ExecuteTemplate(w, "attendance_logs.html", map[string]interface{}{
		"Logs":    logs,
		"Records": allRecords,
		"Filter":  filter,
		"From":    from,
		"To":      to,
		"Role":    u.Role,
	})
}

func adminAddUserHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isAdmin(u) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "add_user.html", map[string]interface{}{"IsSuperAdmin": isSuperAdmin(u)})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")
	if username == "" || password == "" || role == "" {
		templates.ExecuteTemplate(w, "add_user.html", map[string]interface{}{
			"Error":        "All fields are required",
			"IsSuperAdmin": isSuperAdmin(u),
		})
		return
	}
	// Only superadmin can create superadmin or admin accounts
	if role == "superadmin" && !isSuperAdmin(u) {
		role = "employee"
	}
	if role == "admin" && !isAdmin(u) {
		role = "employee"
	}
	db.Exec("INSERT INTO users (username, password, role) VALUES (?, ?, ?)", username, password, role)
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func adminDeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isSuperAdmin(u) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	username := r.FormValue("username")
	// Prevent deleting yourself or superadmin
	if username == u.Username || username == "superadmin" {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}
	db.Exec("DELETE FROM users WHERE username=?", username)
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func adminChangeRoleHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isSuperAdmin(u) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	username := r.FormValue("username")
	newRole := r.FormValue("role")
	if username == u.Username || username == "superadmin" {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}
	allowedRoles := map[string]bool{"employee": true, "admin": true, "superadmin": true}
	if !allowedRoles[newRole] {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}
	db.Exec("UPDATE users SET role=? WHERE username=?", newRole, username)
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func exportLogsHandler(w http.ResponseWriter, r *http.Request) {
	u := getSessionUser(r)
	if !isAdmin(u) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	filter := r.URL.Query().Get("filter")

	now := time.Now()
	switch filter {
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		from = now.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")
		to = now.Format("2006-01-02")
	case "monthly":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		to = now.Format("2006-01-02")
	}

	query := `SELECT employee, check_in, check_out, duration, address_in, address_out FROM attendance`
	args := []interface{}{}
	if from != "" && to != "" {
		query += ` WHERE date(check_in) >= ? AND date(check_in) <= ?`
		args = append(args, from, to)
	}
	query += ` ORDER BY check_in DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=attendance_%s.csv", time.Now().Format("20060102_150405")))

	fmt.Fprintf(w, "Employee,Check-In,Check-Out,Duration,Address In,Address Out\n")
	for rows.Next() {
		var rec ShiftRecord
		rows.Scan(&rec.Employee, &rec.CheckIn, &rec.CheckOut, &rec.Duration, &rec.AddressIn, &rec.AddressOut)
		fmt.Fprintf(w, "%s,%s,%s,%s,\"%s\",\"%s\"\n",
			rec.Employee, rec.CheckIn, rec.CheckOut, rec.Duration, rec.AddressIn, rec.AddressOut)
	}
}

/* ── MAIN ───────────────────────────────────────────────────────────────── */

func main() {
	initDB()

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)

	http.HandleFunc("/employee", employeeHandler)
	http.HandleFunc("/submit", submitAttendanceHandler)

	http.HandleFunc("/admin/dashboard", adminDashboardHandler)
	http.HandleFunc("/admin/logs", adminLogsHandler)
	http.HandleFunc("/admin/users/add", adminAddUserHandler)
	http.HandleFunc("/admin/users/delete", adminDeleteUserHandler)
	http.HandleFunc("/admin/users/role", adminChangeRoleHandler)
	http.HandleFunc("/admin/logs/export", exportLogsHandler)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server running on port " + port)
	http.ListenAndServe(":"+port, nil)
}
