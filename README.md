# GoTimeStamp — Railway Deployment Guide

## Project Structure
```
gotimestamp/
├── main.go
├── migrate.go
├── go.mod
├── go.sum
├── Dockerfile
├── railway.toml
├── .gitignore
└── templates/
    ├── login.html
    ├── employee.html
    ├── admin_dashboard.html
    ├── attendance_logs.html
    └── add_user.html
```

## Deploy to Railway (Step-by-Step)

### 1. Push to GitHub
```bash
git init
git add .
git commit -m "Initial commit"
# Create a repo on GitHub, then:
git remote add origin https://github.com/YOUR_USERNAME/gotimestamp.git
git push -u origin main
```

### 2. Deploy on Railway
1. Go to https://railway.app and sign in with GitHub
2. Click **New Project** → **Deploy from GitHub repo**
3. Select your repository
4. Railway auto-detects the Dockerfile and builds it ✅

### 3. Add a Persistent Volume (CRITICAL for SQLite)
1. In your Railway service → click **Volumes** tab
2. Click **Add Volume**
3. Mount path: `/app/data`
4. This keeps your database alive across restarts ✅

### 4. Get Your Public URL
1. In Railway → your service → **Settings** tab
2. Under **Networking** → click **Generate Domain**
3. Your app will be live at: `https://your-app.up.railway.app`

### 5. Keep Alive with UptimeRobot (Free)
1. Sign up at https://uptimerobot.com
2. Click **Add New Monitor**
3. Type: **HTTP(s)**
4. URL: `https://your-app.up.railway.app/health`
5. Monitoring interval: **5 minutes**
6. Click **Create Monitor** ✅

## Default Login
- Username: `admin`
- Password: `admin123`

> ⚠️ Change the admin password immediately after first login!
