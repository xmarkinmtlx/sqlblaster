# Initialize the module
go mod init github.com/xmarkinmtlx/sqlblaster

# Add required dependencies
go get github.com/go-sql-driver/mysql
go get github.com/fatih/color
go get github.com/mitchellh/mapstructure
go get github.com/schollz/progressbar/v3

# Tidy up the dependencies
go mod tidy
