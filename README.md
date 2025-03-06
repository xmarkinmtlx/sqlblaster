                                                                 █                                   
                                                            █████                                   
                                                ████████    ████                                    
                                  ████████    ███████████  █████                                    
                                ███████████  █████  █████  █████                                    
                               █████  █████ █████   █████ ██████       ███                          
                               █████  ████ █████    █████ █████      ██████████████████             
                               ██████ ██   █████    █████ █████      ██████████████████             
                                ███████   █████    ███████████       ██████████████████             
                                 ███████  █████    █████ ████    ██   ████████████                  
                               ███ ███████████    █████████████████  ██████████                     
                             ████   ███████████████████ ███████████  █████                          
                            ████    █████ ███████████  ██████  ████  ████     ██                    
                           █████   ██████  ███████████ ██     █████  ███    █████                   
                            ███████████     █████████████    ███████        ██████████              
                       █████ ████████              █████    ████████ ███████ ████████               
                  █████████████   █████               ██   ██████  █████████ ████████               
                ████████████████  ████              ████    ████  █████ ████████████  ██            
               ████ █████  █████ █████    ████████  ████    ████  █████████ ███████  ███            
             █████  █████  █████ ████   █████████  ██████  █████ █████████ ███ ████████             
             ████  █████  █████  ████ ███████████ ███████  ████  █████   ████  ███████              
            █████  ███████████  ████  ████  ████  ███ █████████████████████                         
             ████████████████   ████ █████ █████ ████ ███████████████████     █████████             
              ███ ████████████ ████  █████ ████ ████████████████      ███████████████               
                  ████    █████████ ████████████████████     ██████████████                         
                 █████    ██████████████████████    █████████████████                               
                 ████    ████████████ ████   ██████████████████                                     
                █████████████         █████████████████                                             
                ████████████   ███████████████████                                                  
               ██████████ ███████████████████                                                       
                      █████████████████                                                             
                            ███████                                                                 

# SQL Blaster: A MySQL Penetration Testing Tool

SQL Blaster is a powerful, feature-rich command-line tool designed for security professionals to test and assess MySQL/MariaDB database servers. From credential brute forcing to complete database exfiltration, SQL Blaster provides a comprehensive suite of penetration testing capabilities.

## Features

- **Credential Testing**
  - Test single username/password pairs
  - Brute force using username and password lists
  - Customizable number of concurrent testing workers
  - Resume support for interrupted testing sessions

- **Interactive Mode**
  - Full-featured MySQL shell with command history
  - Database and table auto-completion
  - Colorized output for better readability
  - Case-sensitive database handling

- **Penetration Testing Helpers**
  - Built-in categorized pentest commands library
  - Examples and templates for common SQL injection techniques
  - User authentication and privilege enumeration
  - File system access commands 

- **Database Enumeration**
  - Automatic privilege, database, and table enumeration
  - Schema extraction
  - Detailed user and permission analysis

- **Complete Data Extraction**
  - Extract all accessible databases to local files
  - Table structure preservation
  - Large table splitting support
  - Progress tracking for large operations

- **Security Features**
  - Dangerous command protection
  - SSL/TLS support with encryption options
  - Secure error handling
  - Comprehensive logging

## Installation

### Prerequisites
- Go 1.16 or higher

### Quick Installation
```bash
# Clone the repository
git clone https://github.com/xmarkinmtlx/sqlblaster.git
cd sqlblaster

# Install all dependencies and build in one command
go mod tidy && go build -o sqlblaster
```

## Alternative Installation Methods
### Using Go Install
```bash
go install github.com/xmarkinmtlx/sqlblaster@latest
```

## Manual Dependencies
### If you prefer to install dependencies manually:
```bash
go get github.com/go-sql-driver/mysql
go get github.com/fatih/color
go get github.com/mitchellh/mapstructure
go get github.com/schollz/progressbar/v3
go build -o sqlblaster
```

## Quick Start
### Basic Authentication Testing
```bash
# Test a single user/password pair
./sqlblaster -h target-server.com -u admin -p password123

# Test multiple users against a password list
./sqlblaster -h target-server.com -U users.txt -P passwords.txt -v
```

## Interactive Mode
```bash
# Start interactive shell after successful login
./sqlblaster -h target-server.com -u admin -p password123 --connect
```

## Database Enumeration
```bash
# Enumerate all accessible databases
./sqlblaster -h target-server.com -u admin -p password123 -Enum

# Save enumeration to file
./sqlblaster -h target-server.com -u admin -p password123 -Enum --enum-output results.txt
```

## Database Extraction
```bash
# Extract all accessible databases
./sqlblaster -h target-server.com -u admin -p password123 --dump --dump-dir ./extracted_data

# Extract with limited output (progress only)
./sqlblaster -h target-server.com -u admin -p password123 --dump --quiet-dump
```

# Advanced Usage
## Configuration Files
### Create a reusable configuration:
```bash
./sqlblaster --generate-config
# Edit the generated config.json file
./sqlblaster --config config.json
```

## SSL/TLS Options
```bash
# Use secure SSL/TLS connection
./sqlblaster -h target-server.com -u admin -p password123 --use-ssl

# Skip SSL verification
./sqlblaster -h target-server.com -u admin -p password123 --skip-ssl
```

## Dangerous Commands
```bash
# Allow potentially dangerous operations
./sqlblaster -h target-server.com -u admin -p password123 --connect --allow-dangerous
```

# Penetration Testing Helpers
### The interactive mode includes a comprehensive MySQL pentest command library. Access it by typing:
```bash
mysql> pentest
```

For specific categories of commands:
```bash
mysql> pentest enumeration
mysql> pentest data_extraction
mysql> pentest authentication
mysql> pentest file_system
mysql> pentest advanced
```

# Command Reference
```vim
Usage: sqlblaster [options]

Options:
  -h <hostname>       Remote MySQL server address (required)
  -u <username>       Single username to test
  -U <username_file>  File containing usernames, one per line
  --port <port>       MySQL server port (default: 3306)
  -p <password>       Single password to test
  -P <password_file>  File containing passwords, one per line
  -v                  Enable verbose mode
  -f                  Stop at first successful login
  --user-first        Loop over all usernames before next password
  -e <command>        MySQL command to execute on success (default: 'SHOW DATABASES;')
  --allow-dangerous   Allow dangerous commands
  --log-file <file>   Log output to a file
  --config <file>     Load settings from a JSON config file
  --use-ssl           Enable SSL/TLS for MySQL connection
  --skip-ssl          Skip SSL/TLS entirely (overrides --use-ssl)
  --workers <number>  Number of concurrent workers (default: 10)
  --generate-config   Generate a sample config file and exit
  --resume            Resume from the last tested credentials
  -Enum               Enumerate privileges, databases, and tables on success
  --enum-output <file> Save enumeration results to a file
  --connect           Enter interactive mode after successful login (requires -u and -p)
  --dump              Dump all databases and tables to files (requires -u and -p)
  --dump-dir <dir>    Directory to save dumped data (default: mysql_dump)
  --quiet-dump        Only show progress during dump, not actual data
  --max-rows <n>      Maximum rows per dump file (default: 10000, 0 for unlimited)
```

# Examples
## Credential Testing
```bash
# Basic login attempt with output capture
./sqlblaster -h mysql.target.com -u admin -p 'P@ssw0rd!' -e 'SHOW DATABASES;' --log-file results.log

# Brute force with multiple credentials
./sqlblaster -h mysql.target.com -U userlist.txt -P passlist.txt -f -v

# Resume interrupted testing
./sqlblaster -h mysql.target.com -U userlist.txt -P passlist.txt --resume
```

## Data Exfiltration
```bash
# Save data from all accessible databases
./sqlblaster -h mysql.target.com -u admin -p 'P@ssw0rd!' --dump --dump-dir ./mysql_data

# Custom extraction with row limit
./sqlblaster -h mysql.target.com -u admin -p 'P@ssw0rd!' --dump --max-rows 5000
```

# Interactive Mode Commands
Once in interactive mode, the following special commands are available:

- help or \h or \? - Display help menu
- exit or quit or \q - Exit interactive mode
- status or \s - Display connection information
- pentest or \p - Show penetration testing commands
- pentest <category> - Show detailed commands for a specific category
- USE <database> - Switch to specified database
- Standard MySQL commands like SHOW DATABASES, DESCRIBE table, etc.

# Security Considerations
- SQL Blaster should only be used against systems you have explicit permission to test
- The tool implements safeguards to prevent accidental damage, but use caution
- Dangerous operations require the --allow-dangerous flag
- Always consider the legal and ethical implications of security testing

# Contributing
Contributions are welcome! Please feel free to submit a Pull Request.

1.  Fork the repository
2.  Create your feature branch (git checkout -b feature/amazing-feature)
3.  Commit your changes (git commit -m 'Add some amazing feature')
4.  Push to the branch (git push origin feature/amazing-feature)
5.  Open a Pull Request

# Disclaimer
** This tool is provided for educational and professional security testing purposes only. The developers assume no liability for misuse or damage caused by this program. Always obtain proper authorization before testing any system.**
