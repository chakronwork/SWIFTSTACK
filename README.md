# webserver (เว็บเซิร์ฟเวอร์) ⚡

> **A portable, lightweight, XAMPP-like local PHP stack for Windows 10/11 x64.**  
> *ชุดเว็บเซิร์ฟเวอร์แบบพกพา สำหรับงานพัฒนา PHP + MariaDB + Nginx บน Windows ที่เบา เสถียร ปลอดภัย และไม่แตะต้อง Registry หรือ System PATH*

---

## 🇹🇭 คู่มือการใช้งานภาษาไทย

### 🌟 จุดเด่นหลัก
- **พกพาได้ 100% (True Portable)**: ไม่มีการเขียน Registry ของระบบ ไม่เปลี่ยน Global PATH ข้อมูลทั้งหมด (โค้ดเว็บ, ฐานข้อมูล, ล็อก) บันทึกอยู่ใต้โฟลเดอร์โปรแกรมเท่านั้น
- **ปลอดภัยกว่า (Secure by Default)**:
  - ทุก Service ผูกไว้กับ `127.0.0.1` เท่านั้น (ป้องกันไม่ให้อุปกรณ์อื่นในวงแลนเข้าถึง)
  - สร้างรหัสผ่าน root ของ MariaDB แบบสุ่มความปลอดภัยสูงตั้งแต่รันครั้งแรก (บันทึกใน `data/.secrets` และถูกใส่ `.gitignore` อัตโนมัติ)
  - ซ่อน `expose_php = Off`, ป้องกันการเข้าถึงไฟล์ซ่อนอย่าง `.git`, `.env`, `.htaccess`
- **ไม่มีปัญหา Orphan Process**: จัดการ Process ด้วย **Windows Job Objects** ทำให้เมื่อปิดหรือหยุดโปรแกรมหลัก ตัว Nginx, PHP-CGI, MariaDB จะถูกปิดตามทันที 100%
- **เบาและประหยัดทรัพยากร**: ใช้แรมขณะ Idle น้อยกว่า 80MB และกิน CPU แทบจะ 0%

---

### 🚀 เริ่มต้นใช้งานด่วน (Quick Start)

1. **ดาวน์โหลดและแตกไฟล์** `webserver-v0.1.0-windows-x64-portable.zip` ไปยังโฟลเดอร์ที่ต้องการ (เช่น `C:\webserver`)
2. **วินิจฉัยสภาพแวดล้อมของเครื่อง**:
   ```powershell
   .\webserver.exe doctor
   ```
3. **เริ่มต้นระบบครั้งแรก (ดาวน์โหลดและยืนยัน SHA256 components)**:
   ```powershell
   .\webserver.exe init
   ```
4. **เปิดระบบเซิร์ฟเวอร์**:
   ```powershell
   .\webserver.exe start
   ```
   เปิดเบราว์เซอร์ไปที่ `http://localhost` เพื่อดูหน้าต้อนรับและลิงก์ `phpinfo()`!

> ⚠️ **หมายเหตุเกี่ยวกับ Windows SmartScreen**:  
> หาก Windows แสดงหน้าต่างแจ้งเตือน *"Windows protected your PC"* ให้คลิกที่ **"More info" (ข้อมูลเพิ่มเติม)** แล้วกด **"Run anyway" (รันต่อไป)**

---

### 💻 คำสั่ง CLI ที่สำคัญ

| คำสั่ง | คำอธิบาย |
|---|---|
| `webserver init` | ดาวน์โหลด components, ตรวจสอบ SHA256, สร้าง configs และตั้งรหัสผ่าน DB |
| `webserver start [service]` | เริ่มทำงานทุก services หรือระบุตัว (`nginx`, `php`, `mariadb`) |
| `webserver stop [service]` | หยุดการทำงานอย่างปลอดภัย |
| `webserver restart [service]` | สั่งรีสตาร์ท service |
| `webserver status` | แสดงตารางสถานะ, PID, Port และ Error ของแต่ละ service |
| `webserver doctor` | ตรวจสอบความพร้อมของระบบ (พอร์ต, Visual C++, hosts, WebView2) |
| `webserver site add <name> [--https]` | เพิ่ม Virtual Host (เช่น `webserver site add shop.test --https`) |
| `webserver site list` | ดูรายชื่อ Virtual Hosts ทั้งหมด |
| `webserver site remove <name>` | ลบ Virtual Host อย่างปลอดภัย พร้อม Rollback หากคอนฟิกพัง |
| `webserver db password show` | ดูรหัสผ่าน MariaDB root ปัจจุบัน |
| `webserver db password reset` | สุ่มสร้างรหัสผ่าน MariaDB root ใหม่ทันที |
| `webserver logs <service> [-f]` | ดู log แบบสด (`-f` เพื่อเกาะติด) |

---

## 🇬🇧 English Documentation

### 🌟 Key Highlights
- **100% Truly Portable**: No system registry pollution, no global PATH modifications. All database files, websites, logs, and sessions live inside the application folder.
- **Secure by Default**: Services bind strictly to `127.0.0.1`. A cryptographically random MariaDB root password is automatically generated on first run (`data/.secrets`). `expose_php=Off`.
- **Zero Orphan Processes**: Process supervision powered by native **Windows Job Objects** (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`). Killing or stopping the manager instantly tears down all children.
- **Ultra Lightweight**: Dashboard and CLI idle at near 0% CPU with memory footprint < 80MB.

### ⚙️ Architecture & Directory Layout

```text
webserver/
├─ webserver.exe            # Unified Go CLI + Wails GUI application
├─ stack.toml               # Single source of truth configuration
├─ versions.toml            # Official pinned component versions & SHA256 hashes
├─ bin/                     # Extracted binaries (nginx, php-cgi, mariadb, mkcert)
├─ conf/                    # Generated configuration files (nginx.conf, php.ini, my.ini)
├─ conf/vhosts/             # Per-site virtual host configurations (*.conf)
├─ conf/ssl/                # Per-site trusted mkcert certificates and keys
├─ www/                     # Default document root and web projects
├─ data/mysql/              # MariaDB data directory (separated from bin for safe upgrades)
├─ logs/                    # Rotating log files (max 10MB each, 5 backups kept)
├─ tmp/ sessions/ backups/  # Temporary storage, PHP sessions, and database backups
└─ licenses/                # Open-source licenses for all bundled components
```

### 🛡️ Elevated Permissions Policy
`webserver.exe` runs under standard user privileges by default. Elevation (UAC) is requested only on-demand via a minimal, non-blocking helper for:
1. Adding or removing domain mapping entries in `C:\Windows\System32\drivers\etc\hosts`.
2. Installing the local development Root CA certificate with `mkcert -install`.

---

## 📄 Licensing

This project is licensed under the **MIT License**.  
All bundled third-party components (MariaDB, phpMyAdmin, Nginx, PHP, Composer, mkcert) are distributed under their respective open-source licenses (GPLv2, BSD, PHP License). See [THIRD-PARTY-LICENSES.md](file:///c:/Users/first/Documents/dev/webserver/licenses/THIRD-PARTY-LICENSES.md) for details.
