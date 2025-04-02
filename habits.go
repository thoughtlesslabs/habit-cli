package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/term"
)

// Why are there these global variables instead of a simple config type?
// Wherever possible, you should avoid global variables.

// ANSI color codes (using 16-color background for better compatibility)
const ( // Background colors
	squareChar     = "  " // Two spaces for the square content
	squareSeparator = "  "  // Two spaces between squares
	vertSeparator   = " " // Simple space separator for better alignment
)

// ViewMode represents the display mode for the grid
type ViewMode int

const (
	ViewSingleHabit ViewMode = iota // View for a single habit
	ViewAggregate                    // Aggregate view for all habits
)

// Represents a day in the grid view
type GridDay struct {
	Date           time.Time
	CompletedCount int  // Number of habits completed (for aggregate view)
	Done           bool // Whether the specific habit was done (for single view)
	InFuture       bool // Whether this date is in the future
}


// Terminal color support variables
var (
	supportsColor bool
	colorDone     string
	colorCode1    string
	colorCode2    string
	colorCode3    string
	colorEmpty    string
	colorReset    string
	boldText      string
	italicText    string
	accentText    string
	resetText     string
	clearScreen   string
)

// Again, avoid globals. This should be a config.
// If you're going to have globals, have them all at the top, not scattered throughout.
// Having them scattered makes it hard to go from "Ok, I know what this thing is"
// to "Ok let's find out what this function does" and back and forth forever until
// you eventually die, releasing you from the mortal coil that is life. And that could
// be nice, as life is nothing but unrelenting pain and sadness with microscopic windows
// of hope that you think might be good but in the end cause even more pain than
// you had before you had a semblance of hope.
var dataFilePath string

// Define HabitStats type at package level for reuse
type HabitStats struct {
	name          string
	currentStreak int
	longestStreak int
	weeklyRate    float64
	monthlyRate   float64
	yearlyRate    float64
}

type Habit struct {
	Name         string                 `json:"name"`
	ShortName    string                 `json:"short_name"`
	DatesTracked []string               `json:"dates_tracked"`
	ReminderInfo map[string]interface{} `json:"reminder_info"`
}

type DataFile struct {
	Habits []Habit `json:"habits"`
}

// Initialize terminal capabilities based on OS
func init() {
	// Windows Command Prompt doesn't support ANSI colors by default
	// But Windows Terminal and PowerShell 5.1+ do support them
	if runtime.GOOS == "windows" {
		// Try to detect if we're in a capable terminal
		// Simple check: CI environments and Windows Terminal/ConEmu often set these
		_, hasColorTerm := os.LookupEnv("COLORTERM")
		_, hasConEmuANSI := os.LookupEnv("ConEmuANSI")
		_, hasWT_SESSION := os.LookupEnv("WT_SESSION")
		_, hasTERM := os.LookupEnv("TERM")
		
		// If none of these are set, disable colors for Windows
		if !hasColorTerm && !hasConEmuANSI && !hasWT_SESSION && !hasTERM {
        // As we only REALLY use "supportsColor" here, just inline set all of these.
            colorDone = ""
            colorCode1 = ""
            colorCode2 = ""
            colorCode3 = ""
            colorEmpty = ""
            colorReset = ""
            boldText = ""
            italicText = ""
            accentText = ""
            resetText = ""
            clearScreen = ""
        } else {
            colorDone = "\033[48;5;22m"  // Dark green for completed habits
            colorCode1 = "\033[48;5;22m"  // Very dark green for 1 habit
            colorCode2 = "\033[48;5;35m"  // Medium vibrant green for 2 habits
            colorCode3 = "\033[48;5;118m" // Bright neon green for 3+ habits
            colorEmpty = "\033[48;5;240m" // Grey for empty boxes
            colorReset = "\033[0m"
            boldText = "\033[1m"
            italicText = "\033[3m"
            accentText = "\033[36m"
            resetText = "\033[0m"
            clearScreen = "\033[H\033[2J"
        }
	}
	
	// Initialize home directory and data file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error determining home directory:", err)
		os.Exit(1)
	}
	dataFilePath = filepath.Join(homeDir, ".habits_tracker.json")
}

func loadData() (*DataFile, error) {
	df := &DataFile{}
	f, err := os.Open(dataFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// If the file doesn't exist, return an empty data structure
			return df, nil
		}
		return nil, err
	}
	defer f.Close()
	// Check if file is empty before decoding
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() == 0 {
		return df, nil // Return empty data if file is empty
	}
	// Reset file pointer after Stat
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(f).Decode(df); err != nil && err != io.EOF {
		// Handle potential empty file or other JSON errors gracefully
		// If it's just EOF on an empty file, it's okay.
		// If it's another error, return it.
		// This check might be redundant given the size check, but safer.
		return nil, fmt.Errorf("error decoding JSON from %s: %w", dataFilePath, err)
	}
	return df, nil
}

func saveData(df *DataFile) error {
	f, err := os.Create(dataFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(df)
}

// FIXME: This is literally never used.
func suggestShortName(habitName string) string {
	// e.g., take first letter of each word, lowercase, strip non-alphanumeric
	words := strings.Fields(habitName)
	var shortBuilder strings.Builder
	for _, w := range words {
		runes := []rune(w)
		if len(runes) > 0 && unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0]) {
			shortBuilder.WriteRune(unicode.ToLower(runes[0]))
		}
	}
	shortName := shortBuilder.String()
	// Keep alphanumeric and underscore/hyphen
	re := regexp.MustCompile(`[^a-z0-9_-]`)
	shortName = re.ReplaceAllString(shortName, "")
	if shortName == "" { // Handle cases where name has no usable characters
		return "habit"
	}
	return shortName
}

// FIXME: Never used.
func ensureUniqueShortName(df *DataFile, initialShortName string) string {
	shortName := initialShortName
	existingShorts := make(map[string]struct{})
	for _, h := range df.Habits {
		existingShorts[h.ShortName] = struct{}{}
	}
	counter := 2
	for {
		if _, exists := existingShorts[shortName]; !exists {
			break
		}
		shortName = fmt.Sprintf("%s%d", initialShortName, counter)
		counter++
	}
	return shortName
}

func findHabit(df *DataFile, identifier string) (*Habit, int) {
	// identifier can be index (1-based), or name, or short name
	idx, idxErr := strconv.Atoi(identifier)
	if idxErr == nil {
		idx-- // convert to 0-based
		if idx >= 0 && idx < len(df.Habits) {
			return &df.Habits[idx], idx
		}
	}
	// fallback: search by name or short name
	for i := range df.Habits {
		h := &df.Habits[i]
		if strings.EqualFold(h.Name, identifier) || h.ShortName == identifier {
			return h, i
		}
	}
	return nil, -1
}

func commandAdd(args []string, df *DataFile) {
	habitName := strings.TrimSpace(strings.Join(args, " "))
	if habitName == "" {
		fmt.Println("\nError: No habit name provided.")
		fmt.Println("Usage: habits add \"Habit Name\"\n")
		return
	}
	// Check if habit name already exists
	for _, h := range df.Habits {
		if strings.EqualFold(h.Name, habitName) {
			fmt.Printf("\nError: Habit with name '%s' already exists.\n\n", habitName)
			return
		}
	}

	// Remove short name generation
	newHabit := Habit{
		Name:         habitName,
		ShortName:    "", // Empty short name
		DatesTracked: []string{},
		ReminderInfo: make(map[string]interface{}), // Initialize map
	}
	df.Habits = append(df.Habits, newHabit)
	if err := saveData(df); err != nil {
		fmt.Println("\nError saving data:", err, "\n")
	} else {
		fmt.Printf("\nHabit added: '%s'\n\n", habitName)
	}
}

func commandList(df *DataFile) {
	if len(df.Habits) == 0 {
		fmt.Println("\nNo habits found. Add one using 'habits add \"My Habit\"'\n")
		return
	}
	
	// Add extra spacing at the beginning
	fmt.Println()
	
	// Replace boxed header with a left-aligned title
	fmt.Printf("%s📋 Your Habits%s\n", boldText, resetText)
	
	// Pagination settings
	const habitsPerPage = 10
	totalHabits := len(df.Habits)
	totalPages := (totalHabits + habitsPerPage - 1) / habitsPerPage // Ceiling division
	
	// Show habits with pagination if needed
	if totalHabits <= habitsPerPage {
		// Simple case: all habits fit on one page
		fmt.Println()
		displayHabitsPage(df.Habits, 0, habitsPerPage)
		// Add extra spacing at the end
		fmt.Println()
	} else {
		// Multiple pages case: implement pagination
		reader := bufio.NewReader(os.Stdin)
		currentPage := 0
		
		for {
			// Display current page
			startIdx := currentPage * habitsPerPage
			endIdx := startIdx + habitsPerPage
			if endIdx > totalHabits {
				endIdx = totalHabits
			}
			
			displayHabitsPage(df.Habits, startIdx, endIdx)
			
			// Only show page info if there are multiple pages
			if totalPages > 1 {
				fmt.Printf("\n%sPage %d of %d%s", boldText, currentPage+1, totalPages, resetText)
			}
			
			// Just wait for Enter to continue or exit
			if currentPage < totalPages-1 {
				reader.ReadString('\n')
				currentPage++
			} else {
				// Add extra spacing before exiting
				fmt.Println()
				return
			}
			
            // No reason to not clear the screen just because it doesn't have color
            fmt.Print(clearScreen)
			// Add extra spacing at the beginning
			fmt.Println()
			fmt.Printf("%s📋 Your Habits%s\n", boldText, resetText)
		}
	}
}

// TODO: You don't need to write "helper function" everywhere.
// You can just write "Display specific page of habits."

// Helper function to display a specific page of habits
func displayHabitsPage(habits []Habit, startIdx, endIdx int) {
	// Make sure endIdx doesn't exceed habits length
	if endIdx > len(habits) {
		endIdx = len(habits)
	}
	
	for i := startIdx; i < endIdx; i++ {
		h := habits[i]
		fmt.Printf("  %s%d.%s %s (%s%s%s)\n", boldText, i+1, resetText, h.Name, italicText, h.ShortName, resetText)
	}
	// Add an extra line break at the end of the list
	if endIdx > startIdx {
		fmt.Println()
	}
}

func commandDone(args []string, df *DataFile) {
	if len(args) == 0 {
		fmt.Println("\nError: Specify which habit to mark as done.")
		fmt.Println("Usage: habits done <index|name|short_name> [--date YYYY-MM-DD]\n")
		return
	}
	
	// Initialize flag set
	doneCmd := flag.NewFlagSet("done", flag.ExitOnError)
	dateFlag := doneCmd.String("date", "", "Date to mark habit as done (YYYY-MM-DD). Defaults to today.")
	// Add short form flag as an alias
	dShortFlag := doneCmd.String("d", "", "Short form for --date")
	
	// Set usage message
	doneCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage: %s done <index|name|short_name> [--date YYYY-MM-DD] or [-d YYYY-MM-DD]\n", os.Args[0])
		doneCmd.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
	}
	
	// Get the habit identifier from the first argument
	identifier := args[0]
	
	// Split args into identifier and flag args
	var flagArgs []string
	flagArgs = args[1:]
	
	// Parse flags from the args after the identifier
	err := doneCmd.Parse(flagArgs)
	if err != nil {
		// Error handled by flag.ExitOnError
		return
	}
	
	// Determine the target habit
	targetHabit, _ := findHabit(df, identifier)
	
	if targetHabit == nil {
		fmt.Printf("\nError: No habit found matching '%s'. Use 'habits list' to see available habits.\n\n", identifier)
		checkReminders(df)
		return
	}
	
	// Determine target date
	targetDate := time.Now()
	
	// Use the date flag if provided (prefer long form, fallback to short form)
	dateValue := *dateFlag
	if dateValue == "" {
		dateValue = *dShortFlag // Use the short form if long form is empty
	}
	
	if dateValue != "" {
		var err error
		targetDate, err = time.Parse("2006-01-02", dateValue)
		if err != nil {
			fmt.Printf("\nError: Invalid date format '%s'. Use YYYY-MM-DD format.\n\n", dateValue)
			return
		}
		
		// Check if date is in the future
		now := time.Now()
		if targetDate.After(now) {
			fmt.Printf("\nError: Cannot mark habit as done for future date '%s'.\n\n", dateValue)
			return
		}
	}
	
	// Format the date to YYYY-MM-DD
	dateStr := targetDate.Format("2006-01-02")
	
	// Check if already completed on this date
	for _, d := range targetHabit.DatesTracked {
		if d == dateStr {
			fmt.Printf("\n'%s' was already marked as done for %s.\n\n", targetHabit.Name, dateStr)
			return
		}
	}
	
	// Add date to tracked dates
	targetHabit.DatesTracked = append(targetHabit.DatesTracked, dateStr)
	
	// Sort dates for consistency and better streak calculations
	sort.Strings(targetHabit.DatesTracked)
	
	// Save updated data
	if err := saveData(df); err != nil {
		fmt.Println("\nError saving data:", err, "\n")
		return
	}
	
	fmt.Println() // Add spacing before output
	fmt.Printf("Marked '%s' as done for %s!\n", targetHabit.Name, dateStr)
	
	// Output streak info
	currentStreak := calculateStreak(targetHabit.DatesTracked, true)
	if currentStreak > 1 {
		fmt.Printf("Current streak: %d days! 🔥\n", currentStreak)
	}
	
	fmt.Println() // Add spacing after output
}

func commandDelete(args []string, df *DataFile) {
	if len(args) == 0 {
		fmt.Println("\nError: Specify which habit to delete.")
		fmt.Println("Usage: habits delete <index|name|short_name>\n")
		return
	}
	identifier := strings.Join(args, " ")
	habit, index := findHabit(df, identifier)
	if habit == nil {
		fmt.Printf("\nError: No habit found matching '%s'.\n\n", identifier)
		return
	}
	
	fmt.Println() // Add spacing before prompting
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Are you sure you want to delete habit '%s'? (y/n): ", habit.Name)
	resp, _ := reader.ReadString('\n')
	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp == "y" || resp == "yes" {
		if index >= 0 && index < len(df.Habits) {
			// Save the habit name before deletion
			habitName := habit.Name
			df.Habits = append(df.Habits[:index], df.Habits[index+1:]...)
			if err := saveData(df); err != nil {
				fmt.Println("Error saving data:", err)
			} else {
				fmt.Printf("Habit '%s' deleted.\n\n", habitName)
			}
		} else {
			// This should technically not happen if findHabit returned a non-nil habit
			fmt.Println("Error: Could not delete habit due to index issue.\n")
		}
	} else {
		fmt.Println("Deletion canceled.\n")
	}
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // Default width if detection fails
	}
	return width
}

// Calculates the start date (a Sunday) for the grid, ensuring today is included
func calculateStartDate() time.Time {
	today := time.Now()
	
	// Determine how many weeks to go back from today
	weeksToGoBack := 52
	
	// Go back 52 weeks (364 days) as a starting point
	oneYearAgo := today.AddDate(0, 0, -(weeksToGoBack*7))
	
	// Calculate day of week of oneYearAgo (0 = Sunday, 6 = Saturday)
	dayOfWeek := int(oneYearAgo.Weekday())
	
	// Find the Sunday that starts the week containing oneYearAgo
	// If it's already Sunday (dayOfWeek == 0), don't adjust
	startDate := oneYearAgo
	if dayOfWeek != 0 {
		// Go back to the previous Sunday
		startDate = oneYearAgo.AddDate(0, 0, -dayOfWeek)
	}
	
	// Make sure we include at least this week
	endDate := startDate.AddDate(0, 0, weeksToGoBack*7)
	todayYearDay := today.YearDay()
	endDateYearDay := endDate.YearDay()
	
	// If the end date doesn't reach today, adjust the start date
	if endDate.Year() < today.Year() || (endDate.Year() == today.Year() && endDateYearDay < todayYearDay) {
		// Calculate how many more days we need
		daysToAdd := 0
		if endDate.Year() < today.Year() {
			// Different years, more complex calculation
			daysInEndYear := 365
			if isLeapYear(endDate.Year()) {
				daysInEndYear = 366
			}
			daysToAdd = (daysInEndYear - endDateYearDay) + todayYearDay
		} else {
			// Same year, simple subtraction
			daysToAdd = todayYearDay - endDateYearDay
		}
		
		// Add a week to ensure we include today
		daysToAdd += 7
		
		// Adjust the start date accordingly
		startDate = startDate.AddDate(0, 0, daysToAdd)
	}
	
	return startDate
}

// Helper function to check if a year is a leap year
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// printGrid prints a simple grid to the console
func printGrid(days []GridDay, mode ViewMode, width int, singleHabitName string) {
	if len(days) == 0 {
		fmt.Println("No tracking data found.")
		return
	}

	// Define box characters and dimensions
	const boxWidth = 2      // Width of each box in characters
	const boxSpacing = 1    // Space between boxes
	const cellWidth = boxWidth + boxSpacing // Total width of one cell including spacing

	// Calculate how many boxes can fit in a line
	squaresPerLine := width / cellWidth
	if squaresPerLine <= 0 {
		squaresPerLine = 10 // Even narrower fallback for extremely small terminals
	}

	fmt.Printf("\n")
	
	// Print rows of squares
	for i := 0; i < len(days); i += squaresPerLine {
		end := i + squaresPerLine
		if end > len(days) {
			end = len(days)
		}
		
		// Print a row of squares
		for j := i; j < end; j++ {
			if days[j].InFuture {
				// Future days are shown as dots with consistent spacing
				fmt.Print("·· ")
				continue
			}
			
			// Determine display based on mode and completion count
			if mode == ViewSingleHabit {
				// Single habit view - binary done/not done
				if days[j].Done {
					fmt.Print(colorDone + squareChar + colorReset + " ")
				} else {
					fmt.Print(colorEmpty + squareChar + colorReset + " ")
				}
			} else {
				// Aggregate view - color based on count
				switch days[j].CompletedCount {
				case 0:
					fmt.Print(colorEmpty + squareChar + colorReset + " ")
				case 1:
					fmt.Print(colorCode1 + squareChar + colorReset + " ")
				case 2:
					fmt.Print(colorCode2 + squareChar + colorReset + " ")
				default: // 3+
					fmt.Print(colorCode3 + squareChar + colorReset + " ")
				}
			}
		}
		// End of row - add proper spacing
		fmt.Println()
		fmt.Println() // Double spacing between rows for better readability
	}
	
	// Print legend
	fmt.Println()
	if mode == ViewSingleHabit {
		fmt.Println("Legend: " + colorEmpty + squareChar + colorReset + " Not Done    " + 
		    colorDone + squareChar + colorReset + " Done")
	} else {
		fmt.Println("Legend: " + colorEmpty + squareChar + colorReset + " None    " + 
		    colorCode1 + squareChar + colorReset + " 1 habit    " + 
			colorCode2 + squareChar + colorReset + " 2 habits    " + 
			colorCode3 + squareChar + colorReset + " 3+ habits")
	}
}

func commandView(args []string, df *DataFile) {
	// Define flag set for view command
	viewCmd := flag.NewFlagSet("view", flag.ExitOnError)
	rangeFlag := viewCmd.String("range", "last30", "View range: year, month, week, day, last30")
	// Add short form flag as an alias
	rShortFlag := viewCmd.String("r", "", "Short form for --range")
	
	// Set usage message
	viewCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s tracker <id> [--range <range>] or [-r <range>]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Range options: year, month, week, day, last30\n")
		viewCmd.PrintDefaults()
	}
	
	// Find the habit identifier and remaining flags
	var identifier string
	var flagArgs []string
	
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		identifier = args[0]
		flagArgs = args[1:]
	} else {
		flagArgs = args
	}
	
	// Parse flags
	err := viewCmd.Parse(flagArgs)
	if err != nil {
		return
	}
	
	// Get range value (prefer long form, fallback to short form)
	viewRange := *rangeFlag
	if viewRange == "last30" && *rShortFlag != "" {
		viewRange = *rShortFlag
	}
	
	// Find the habit
	habit, _ := findHabit(df, identifier)
	if habit == nil {
		fmt.Printf("Error: No habit found matching '%s'.\n", identifier)
		return
	}
	
    fmt.Print(clearScreen)
	fmt.Printf("📊 %sTracker: %s%s (%s%s%s)\n\n", boldText, habit.Name, resetText, italicText, habit.ShortName, resetText)
	
	// If day view, show the daily summary instead of grid
	if viewRange == "day" {
		showDayView(df, habit)
		return
	}

	completedDates := make(map[string]bool)
	for _, d := range habit.DatesTracked {
		completedDates[d] = true
	}
	
	// Determine time range based on viewRange
	var numWeeks int
	var startDate time.Time
	
	switch viewRange {
	case "year":
		numWeeks = 52
		startDate = calculateStartDate()
	case "month":
		numWeeks = 5 // Enough weeks to show a month
		startDate = calculateMonthStartDate()
	case "week":
		numWeeks = 1
		startDate = calculateWeekStartDate()
	case "last30":
		numWeeks = 5 // 5 weeks to ensure 30 days
		startDate = calculateLast30DaysStartDate()
	}
	
	// Generate grid data for a single habit
	gridData := make([]GridDay, 0, numWeeks*7)
	currentDate := startDate
	
	// Create a flat list of GridDay entries for the selected time period
	for i := 0; i < numWeeks*7; i++ {
		dateStr := currentDate.Format("2006-01-02")
		day := GridDay{
			Date:     currentDate,
			Done:     completedDates[dateStr],
			InFuture: currentDate.After(time.Now()),
		}
		gridData = append(gridData, day)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	printGrid(gridData, ViewSingleHabit, getTerminalWidth(), habit.Name)
}

// Helper function to calculate start date for month view (first day of current month)
func calculateMonthStartDate() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}

// Helper function to calculate start date for week view (previous Sunday)
func calculateWeekStartDate() time.Time {
	now := time.Now()
	dayOfWeek := int(now.Weekday())
	
	// Go back to previous Sunday (or today if it's Sunday)
	return now.AddDate(0, 0, -dayOfWeek)
}

// Helper function to calculate start date for last 30 days view
func calculateLast30DaysStartDate() time.Time {
	now := time.Now()
	return now.AddDate(0, 0, -29) // 30 days including today
}

// Helper function to show the day view (list of habits with completion status)
func showDayView(df *DataFile, specificHabit *Habit) {
	today := time.Now().Format("2006-01-02")
	fmt.Printf("Today: %s\n\n", today)
	
	if specificHabit != nil {
		// Show just the specific habit
		isDone := false
		for _, d := range specificHabit.DatesTracked {
			if d == today {
				isDone = true
				break
			}
		}
		
		if isDone {
			fmt.Printf("  %s %s\n", colorDone+squareChar+colorReset, specificHabit.Name)
		} else {
			fmt.Printf("  %s %s\n", colorEmpty+squareChar+colorReset, specificHabit.Name)
		}
	} else {
		// Show all habits
		for i, habit := range df.Habits {
			isDone := false
			for _, d := range habit.DatesTracked {
				if d == today {
					isDone = true
					break
				}
			}
			
			if isDone {
				fmt.Printf("  %s %s\n", colorDone+squareChar+colorReset, habit.Name)
			} else {
				fmt.Printf("  %s %s\n", colorEmpty+squareChar+colorReset, habit.Name)
			}
			
			// Add an extra line between habits for visual separation
			if i < len(df.Habits)-1 {
				fmt.Println()
			}
		}
	}
	
	// Show legend
	fmt.Println()
	fmt.Println("Legend: " + colorEmpty + squareChar + colorReset + " Not Done    " + 
		colorDone + squareChar + colorReset + " Done")
}

func commandViewAggregate(df *DataFile, viewRange string) {
	if len(df.Habits) == 0 {
		fmt.Println("No habits to view.")
		return
	}
	
    fmt.Print(clearScreen)
	fmt.Printf("📊 %sTracker%s\n\n", boldText, resetText)

	// Calculate daily completion counts for all habits
	dailyCounts := make(map[string]int)
	for _, habit := range df.Habits {
		for _, dateStr := range habit.DatesTracked {
			// No need to filter dates here
			dailyCounts[dateStr]++
		}
	}
	
	// If day view, show the daily summary instead of grid
	if viewRange == "day" {
		showDayView(df, nil)
		return
	}
	
	// Show today's date and completion stats (replacing debug output)
	todayStr := time.Now().Format("2006-01-02")
	totalCompletedToday := dailyCounts[todayStr]
	totalHabits := len(df.Habits)
	fmt.Printf("Today is %s - Completed: %d/%d habits\n\n", todayStr, totalCompletedToday, totalHabits)

	// Determine time range based on viewRange
	var numWeeks int
	var startDate time.Time
	
	switch viewRange {
	case "year":
		numWeeks = 52
		startDate = calculateStartDate()
	case "month":
		numWeeks = 5 // Enough weeks to show a month
		startDate = calculateMonthStartDate()
	case "week":
		numWeeks = 1
		startDate = calculateWeekStartDate()
	case "last30":
		numWeeks = 5 // 5 weeks to ensure 30 days
		startDate = calculateLast30DaysStartDate()
	}
	
	// Generate grid data for all habits
	gridData := make([]GridDay, 0, numWeeks*7)
	currentDate := startDate
	
	// Create a flat list of GridDay entries for the selected time period
	for i := 0; i < numWeeks*7; i++ {
		dateStr := currentDate.Format("2006-01-02")
		day := GridDay{
			Date:           currentDate,
			CompletedCount: dailyCounts[dateStr],
			InFuture:       currentDate.After(time.Now()),
		}
		gridData = append(gridData, day)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	printGrid(gridData, ViewAggregate, getTerminalWidth(), "")
}

func checkReminders(df *DataFile) []string {
	today := time.Now().Format("2006-01-02")
	needsReminder := []string{}
	for _, h := range df.Habits {
		isDoneToday := false
		for _, d := range h.DatesTracked {
			if d == today {
				isDoneToday = true
				break
			}
		}
		if !isDoneToday {
			needsReminder = append(needsReminder, h.Name)
		}
	}

	return needsReminder
}

// New function that returns habit indices and names
func checkRemindersWithIndices(df *DataFile) [][2]string {
	today := time.Now().Format("2006-01-02")
	needsReminder := [][2]string{}
	for i, h := range df.Habits {
		isDoneToday := false
		for _, d := range h.DatesTracked {
			if d == today {
				isDoneToday = true
				break
			}
		}
		if !isDoneToday {
			// Store both the index (1-based) and name
            // TODO: Why is the index 1-based?
			needsReminder = append(needsReminder, [2]string{strconv.Itoa(i+1), h.Name})
		}
	}

	return needsReminder
}

func printReminders(needsReminder []string) {
	if len(needsReminder) > 0 {
		fmt.Println("📝 Habits due today:")
		for _, habitName := range needsReminder {
			fmt.Printf("  • %s\n", habitName)
		}
		fmt.Println()
	}
}

func calculateStreak(dates []string, isCurrentStreak bool) int {
	if len(dates) == 0 {
		return 0
	}

	// Parse and sort dates
	parsed := make([]time.Time, 0, len(dates))
	for _, d := range dates {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue // Skip invalid dates
		}
		parsed = append(parsed, t)
	}

	// Sort dates in ascending order
	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].Before(parsed[j])
	})

	// Current streak: starts from the most recent date and goes backward
	// Longest streak: finds the longest consecutive sequence
	
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	
	if isCurrentStreak {
		// Check if the most recent date is today or yesterday
		if len(parsed) == 0 || (parsed[len(parsed)-1].Before(yesterday)) {
			return 0 // No current streak if most recent date is before yesterday
		}
		
		streak := 1
		currentDate := parsed[len(parsed)-1]
		
		// Count consecutive days backward
		for i := len(parsed) - 2; i >= 0; i-- {
			expectedDate := currentDate.AddDate(0, 0, -1)
			if expectedDate.Equal(parsed[i]) {
				streak++
				currentDate = parsed[i]
			} else {
				break
			}
		}
		return streak
	} else {
		// Find longest streak
		if len(parsed) == 0 {
			return 0
		}
		
		maxStreak := 1
		currentStreak := 1
		
		for i := 1; i < len(parsed); i++ {
			expectedDate := parsed[i-1].AddDate(0, 0, 1)
			if expectedDate.Equal(parsed[i]) {
				currentStreak++
			} else {
				// Streak broken
				if currentStreak > maxStreak {
					maxStreak = currentStreak
				}
				currentStreak = 1
			}
		}
		
		// Check if the final streak is the longest
		if currentStreak > maxStreak {
			maxStreak = currentStreak
		}
		
		return maxStreak
	}
}

// Add a yearly calculation period
func calculateCompletionRate(dates []string, period int) float64 {
	if len(dates) == 0 {
		return 0.0
	}
	
	// Parse dates and count unique dates within the period
	uniqueDates := make(map[string]bool)
	for _, d := range dates {
		uniqueDates[d] = true
	}
	
	// Calculate completion rate over the specified period
	today := time.Now()
	startDate := today.AddDate(0, 0, -period+1) // +1 to include today
	
	totalDays := 0
	completedDays := 0
	
	for d := startDate; !d.After(today); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		totalDays++
		if uniqueDates[dateStr] {
			completedDays++
		}
	}
	
	if totalDays == 0 {
		return 0.0
	}
	
	return float64(completedDays) / float64(totalDays) * 100
}

func commandStats(args []string, df *DataFile) {
	// Determine if we're showing stats for a specific habit or all habits
	var specificHabit *Habit = nil
	
	if len(args) > 0 {
		identifier := strings.Join(args, " ")
		specificHabit, _ = findHabit(df, identifier)
		if specificHabit == nil {
			fmt.Printf("Error: No habit found matching '%s'.\n", identifier)
			return
		}
	}
	
	// For a specific habit
	if specificHabit != nil {
		fmt.Printf("%s📊 Statistics for '%s'%s\n\n", boldText, specificHabit.Name, resetText)
	} else {
		// For all habits
		fmt.Printf("%s📊 Habit Statistics%s\n", boldText, resetText)
	}
	
	// If showing stats for a single habit
	if specificHabit != nil {
		// Display single habit stats (unchanged)
		dates := specificHabit.DatesTracked
		currentStreak := calculateStreak(dates, true)
		longestStreak := calculateStreak(dates, false)
		weeklyRate := calculateCompletionRate(dates, 7)
		monthlyRate := calculateCompletionRate(dates, 30)
		yearlyRate := calculateCompletionRate(dates, 365)
		
		fmt.Printf("  %sCurrent Streak:%s %d day(s)\n", boldText, resetText, currentStreak)
		fmt.Printf("  %sLongest Streak:%s %d day(s)\n", boldText, resetText, longestStreak)
		fmt.Printf("  %sTotal Completions:%s %d time(s)\n", boldText, resetText, len(dates))
		fmt.Printf("  %sCompletion Rate:%s\n", boldText, resetText)
		fmt.Printf("    • Last 7 days: %.1f%% (%d of 7 days)\n", 
			weeklyRate, int(weeklyRate * 7 / 100))
		fmt.Printf("    • Last 30 days: %.1f%% (%d of 30 days)\n", 
			monthlyRate, int(monthlyRate * 30 / 100))
		fmt.Printf("    • Last 365 days: %.1f%% (%d of 365 days)\n", 
			yearlyRate, int(yearlyRate * 365 / 100))
		
		// Show graph at the end
		fmt.Println()
		// Use the non-clearing tracker function
		showTrackerWithoutClearing([]string{specificHabit.Name, "--range", "last30"}, df)
	} else {
		// Collect stats for all habits
		fmt.Println()
		fmt.Printf("  %sHabit Summary:%s\n\n", boldText, resetText)
		
		// Remove the initial table header that causes duplication
		
		// Sort habits by current streak (descending)
		allStats := make([]HabitStats, 0, len(df.Habits))
		
		for _, h := range df.Habits {
			currentStreak := calculateStreak(h.DatesTracked, true)
			longestStreak := calculateStreak(h.DatesTracked, false)
			weeklyRate := calculateCompletionRate(h.DatesTracked, 7)
			monthlyRate := calculateCompletionRate(h.DatesTracked, 30)
			yearlyRate := calculateCompletionRate(h.DatesTracked, 365)
			
			allStats = append(allStats, HabitStats{
				name:          h.Name,
				currentStreak: currentStreak,
				longestStreak: longestStreak,
				weeklyRate:    weeklyRate,
				monthlyRate:   monthlyRate,
				yearlyRate:    yearlyRate,
			})
		}
		
		// Sort by current streak (descending)
		sort.Slice(allStats, func(i, j int) bool {
			return allStats[i].currentStreak > allStats[j].currentStreak
		})
		
		// Pagination settings
		const statsPerPage = 10
		totalStats := len(allStats)
		totalPages := (totalStats + statsPerPage - 1) / statsPerPage // Ceiling division
		
		// Show stats with pagination if needed
		if totalStats <= statsPerPage {
			// Simple case: all stats fit on one page
			// Add the table header here for the single page case
			fmt.Printf("  %-25s %10s %10s %12s %12s %12s\n", 
				"HABIT", "STREAK", "LONGEST", "WEEK", "MONTH", "YEAR")
			fmt.Println("  " + strings.Repeat("─", 85))
			displayStatsPage(allStats, 0, totalStats)
		} else {
			// Multiple pages case: implement pagination
			reader := bufio.NewReader(os.Stdin)
			currentPage := 0
			
			for {
				// Display current page
				startIdx := currentPage * statsPerPage
				endIdx := startIdx + statsPerPage
				if endIdx > totalStats {
					endIdx = totalStats
				}
				
				// Re-print the table header for each page
				fmt.Printf("  %-25s %10s %10s %12s %12s %12s\n", 
					"HABIT", "STREAK", "LONGEST", "WEEK", "MONTH", "YEAR")
				fmt.Println("  " + strings.Repeat("─", 85))
				
				displayStatsPage(allStats, startIdx, endIdx)
				
				// Only show page info if there are multiple pages
				if totalPages > 1 {
					fmt.Printf("\n\033[1mPage %d of %d\033[0m", currentPage+1, totalPages)
				}
				
				// Just wait for Enter to continue or exit
				if currentPage < totalPages-1 {
					reader.ReadString('\n')
					currentPage++
				} else {
					// Inform user how to view the aggregate view on exit
					fmt.Println()
					fmt.Println("Use 'habits tracker' to see the aggregate habit view.")
					return
				}
				
				// Clear screen between pages for better readability
				fmt.Print("\033[H\033[2J") // Clear screen
				fmt.Println("\033[1m📊 Habit Statistics\033[0m")
				fmt.Println()
				fmt.Printf("  %sHabit Summary:%s\n\n", boldText, resetText)
			}
		}
		
		// Inform user how to view the aggregate view
		fmt.Println()
		fmt.Println("Use 'habits tracker' to see the aggregate habit view.")
	}
}

// Helper function to display a specific page of habit stats
func displayStatsPage(stats []HabitStats, startIdx, endIdx int) {
	// Make sure endIdx doesn't exceed stats length
	if endIdx > len(stats) {
		endIdx = len(stats)
	}
	
	for i := startIdx; i < endIdx; i++ {
		stat := stats[i]
		name := stat.name
		if len(name) > 22 {
			name = name[:19] + "..."
		}
		weekStr := fmt.Sprintf("%d/7 days", int(stat.weeklyRate * 7 / 100)) 
		monthStr := fmt.Sprintf("%d/30 days", int(stat.monthlyRate * 30 / 100))
		yearStr := fmt.Sprintf("%d/365 days", int(stat.yearlyRate * 365 / 100))
		
		fmt.Printf("  %-25s %10d %10d %12s %12s %12s\n",
			name, stat.currentStreak, stat.longestStreak, weekStr, monthStr, yearStr)
	}
}

func commandEdit(args []string, df *DataFile) {
	if len(args) < 1 {
		fmt.Println("Error: Specify which habit to edit.")
		fmt.Println("Usage: habits edit <id> [--name \"New Name\"] [--short \"new_short\"]")
		return
	}
	
	// Use flagSet for 'edit' command
	editCmd := flag.NewFlagSet("edit", flag.ExitOnError)
	newName := editCmd.String("name", "", "New name for the habit")
	newShort := editCmd.String("short", "", "New short name for the habit")
	// Add short form flags as aliases
	nShortFlag := editCmd.String("n", "", "Short form for --name")
	sShortFlag := editCmd.String("s", "", "Short form for --short")
	
	// Set usage message
	editCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s edit <index|name|short_name> [--name \"New Name\"] [--short \"new_short\"]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  or: %s edit <index|name|short_name> [-n \"New Name\"] [-s \"new_short\"]\n", os.Args[0])
		editCmd.PrintDefaults()
	}
	
	// Find the last non-flag argument (habit identifier)
	var identifier string
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			// Parse remaining flags
			err := editCmd.Parse(args[i:])
			if err != nil {
				return // Error handled by flag.ExitOnError
			}
			break
		}
		if i == len(args)-1 {
			identifier = strings.Join(args, " ")
			// No flags present
			editCmd.Parse([]string{})
		} else {
			identifier += arg + " "
		}
	}
	
	identifier = strings.TrimSpace(identifier)
	habit, index := findHabit(df, identifier)
	
	if habit == nil {
		fmt.Printf("Error: No habit found matching '%s'.\n", identifier)
		return
	}
	
	// Get name value (prefer long form, fallback to short form)
	nameValue := *newName
	if nameValue == "" {
		nameValue = *nShortFlag
	}
	
	// Get short name value (prefer long form, fallback to short form)
	shortValue := *newShort
	if shortValue == "" {
		shortValue = *sShortFlag
	}
	
	// Check if at least one edit option was provided
	if nameValue == "" && shortValue == "" {
		fmt.Println("Error: Specify at least one change (--name/--short or -n/-s).")
		editCmd.Usage()
		return
	}
	
	// Handle name change
	if nameValue != "" {
		// Check if the new name already exists
		for i, h := range df.Habits {
			if i != index && strings.EqualFold(h.Name, nameValue) {
				fmt.Printf("Error: Habit with name '%s' already exists.\n", nameValue)
				return
			}
		}
		
		oldName := habit.Name
		habit.Name = nameValue
		fmt.Printf("Habit name changed from '%s' to '%s'\n", oldName, nameValue)
	}
	
	// Handle short name change
	if shortValue != "" {
		// Validate short name
		re := regexp.MustCompile(`^[a-z0-9_-]+$`)
		if !re.MatchString(shortValue) {
			fmt.Println("Error: Short name must only contain lowercase letters, numbers, underscores and hyphens.")
			return
		}
		
		// Check if the new short name already exists
		for i, h := range df.Habits {
			if i != index && h.ShortName == shortValue {
				fmt.Printf("Error: Habit with short name '%s' already exists.\n", shortValue)
				return
			}
		}
		
		oldShort := habit.ShortName
		habit.ShortName = shortValue
		fmt.Printf("Habit short name changed from '%s' to '%s'\n", oldShort, shortValue)
	}
	
	// Save changes
	if err := saveData(df); err != nil {
		fmt.Println("Error saving data:", err)
	}
}

func commandExport(args []string, df *DataFile) {
	if len(df.Habits) == 0 {
		fmt.Println("No habits to export.")
		return
	}
	
	// Use flagSet for 'export' command
	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	outputFile := exportCmd.String("file", "", "Output file path (defaults to habits_export_<date>.json)")
	// Add short form flag as an alias
	fShortFlag := exportCmd.String("f", "", "Short form for --file")
	
	// Set usage message
	exportCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s export [--file path/to/export.json] or [-f path/to/export.json]\n", os.Args[0])
		exportCmd.PrintDefaults()
	}
	
	// Parse arguments
	err := exportCmd.Parse(args)
	if err != nil {
		return // Error handled by flag.ExitOnError
	}
	
	// Get file value (prefer long form, fallback to short form)
	fileValue := *outputFile
	if fileValue == "" {
		fileValue = *fShortFlag
	}
	
	// Determine output file path
	filePath := fileValue
	if filePath == "" {
		timestamp := time.Now().Format("2006-01-02")
		filePath = fmt.Sprintf("habits_export_%s.json", timestamp)
	}
	
	// Export the data
	f, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("Error creating export file: %v\n", err)
		return
	}
	defer f.Close()
	
	data, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling data: %v\n", err)
		return
	}
	
	_, err = f.Write(data)
	if err != nil {
		fmt.Printf("Error writing data: %v\n", err)
		return
	}
	
	fmt.Printf("Data exported to %s\n", filePath)
}

func commandImport(args []string, df *DataFile) {
	// Use flagSet for 'import' command
	importCmd := flag.NewFlagSet("import", flag.ExitOnError)
	inputFile := importCmd.String("file", "", "Input file path (required)")
	merge := importCmd.Bool("merge", false, "Merge with existing habits instead of replacing")
	// Add short form flags as aliases
	fShortFlag := importCmd.String("f", "", "Short form for --file")
	mShortFlag := importCmd.Bool("m", false, "Short form for --merge")
	
	// Set usage message
	importCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s import --file path/to/import.json [--merge]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  or: %s import -f path/to/import.json [-m]\n", os.Args[0])
		importCmd.PrintDefaults()
	}
	
	// Parse arguments
	err := importCmd.Parse(args)
	if err != nil {
		return // Error handled by flag.ExitOnError
	}
	
	// Get file value (prefer long form, fallback to short form)
	fileValue := *inputFile
	if fileValue == "" {
		fileValue = *fShortFlag
	}
	
	// Get merge value (either long or short form)
	mergeValue := *merge || *mShortFlag
	
	// Validate file path
	if fileValue == "" {
		fmt.Println("Error: No input file specified")
		importCmd.Usage()
		return
	}
	
	// Read the import file
	data, err := os.ReadFile(fileValue)
	if err != nil {
		fmt.Printf("Error reading import file: %v\n", err)
		return
	}
	
	// Parse the JSON data
	var importedData DataFile
	err = json.Unmarshal(data, &importedData)
	if err != nil {
		fmt.Printf("Error parsing JSON data: %v\n", err)
		return
	}
	
	// Process the imported data
	if mergeValue {
		// Merge with existing data
		existingHabits := make(map[string]bool)
		for _, h := range df.Habits {
			existingHabits[h.Name] = true
		}
		
		// Add only new habits
		for _, h := range importedData.Habits {
			if !existingHabits[h.Name] {
				df.Habits = append(df.Habits, h)
			}
		}
		
		fmt.Printf("Merged %d new habits from %s\n", len(importedData.Habits), fileValue)
	} else {
		// Replace existing data
		*df = importedData
		fmt.Printf("Imported %d habits from %s\n", len(importedData.Habits), fileValue)
	}
	
	// Save the updated data
	if err := saveData(df); err != nil {
		fmt.Println("Error saving data:", err)
	}
}

func commandUndone(df *DataFile) {
	// Use the new function that preserves indices
	needsReminder := checkRemindersWithIndices(df)
	if len(needsReminder) > 0 {
		fmt.Println("Habits not yet completed today:")
		for _, habit := range needsReminder {
			index, name := habit[0], habit[1]
			fmt.Printf("  \033[1m%s.\033[0m %s\n", index, name)
		}
		fmt.Println()
	} else if len(df.Habits) == 0 {
		fmt.Println("No habits to track.")
	} else {
		fmt.Println("All habits completed for today! 🎉")
	}
}

// New function: commandRemove implements what undone used to do
// FIXME: What did "undone" used to do? Why the change?
func commandRemove(args []string, df *DataFile) {
	if len(args) == 0 {
		fmt.Println("Error: Specify which habit to remove completion for.")
		fmt.Println("Usage: habits remove <index|name|short_name> [--date YYYY-MM-DD]")
		return
	}
	
	// Initialize flag set
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
	dateFlag := removeCmd.String("date", "", "Date to remove completion for (YYYY-MM-DD). Defaults to today.")
	// Add short form flag as an alias
	dShortFlag := removeCmd.String("d", "", "Short form for --date")
	
	// Set usage message
	removeCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s remove <index|name|short_name> [--date YYYY-MM-DD] or [-d YYYY-MM-DD]\n", os.Args[0])
		removeCmd.PrintDefaults()
	}
	
	// Get the habit identifier from the first argument
	identifier := args[0]
	
	// Split args into identifier and flag args
	var flagArgs []string
	flagArgs = args[1:]
	
	// Parse flags from the args after the identifier
	err := removeCmd.Parse(flagArgs)
	if err != nil {
		// Error handled by flag.ExitOnError
		return
	}
	
	// Find the habit
	targetHabit, _ := findHabit(df, identifier)
	
	if targetHabit == nil {
		fmt.Printf("Error: No habit found matching '%s'. Use 'habits list' to see available habits.\n", identifier)
		return
	}
	
	// Determine target date
	targetDate := time.Now()
	
	// Use the date flag if provided (prefer long form, fallback to short form)
	dateValue := *dateFlag
	if dateValue == "" {
		dateValue = *dShortFlag // Use the short form if long form is empty
	}
	
	if dateValue != "" {
		var err error
		targetDate, err = time.Parse("2006-01-02", dateValue)
		if err != nil {
			fmt.Printf("Error: Invalid date format '%s'. Use YYYY-MM-DD format.\n", dateValue)
			return
		}
		
		// Check if date is in the future
		now := time.Now()
		if targetDate.After(now) {
			fmt.Printf("Error: Cannot mark habit as done for future date '%s'.\n", dateValue)
			return
		}
	}
	
	// Format the date to YYYY-MM-DD
	dateStr := targetDate.Format("2006-01-02")
	
	// Check if the date exists in the habit's tracked dates
	found := false
	var newDates []string
	
	for _, d := range targetHabit.DatesTracked {
		if d == dateStr {
			found = true
		} else {
			newDates = append(newDates, d)
		}
	}
	
	if found {
		targetHabit.DatesTracked = newDates
		
		// Save updated data
		if err := saveData(df); err != nil {
			fmt.Println("Error saving data:", err)
			return
		}
		
		fmt.Printf("Removed completion for '%s' on %s.\n", targetHabit.Name, dateStr)
	} else {
		fmt.Printf("'%s' was not marked as done for %s.\n", targetHabit.Name, dateStr)
	}
}

func printHelp() {
	cmdWidth := 30 // Adjust command display width
    
    // Emojis are illegal.
	fmt.Printf("%s Habits Tracker - Help%s\n", boldText, resetText)
	
	fmt.Printf("Usage: %shabits%s <command> [arguments...]\n", boldText, resetText)
	fmt.Printf("\n%sCommands:%s\n", boldText, resetText)
	
	// Basic commands - most commonly used
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "add \"<habit name>\"", resetText, "Add a new habit.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "list", resetText, "List all habits with index and short name.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "tracker [<id>]", resetText, "View habit tracker (aggregate if ID omitted).")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "tracker --range <range>", resetText, "View with range: year, month, week, day, last30.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "undone", resetText, "List all habits not completed today.")
	
	// Tracking commands
	fmt.Printf("\n%sTracking Commands:%s\n", boldText, resetText)
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "done <id>", resetText, "Mark a habit as done for today.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "done <id> -date DATE", resetText, "Mark a habit as done for specific date.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "remove <id>", resetText, "Remove completion for today.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "remove <id> -date DATE", resetText, "Remove completion for specific date.")
	
	// Management commands
	fmt.Printf("\n%sManagement Commands:%s\n", boldText, resetText)
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "stats [<id>]", resetText, "Show statistics (all habits if id omitted).")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "edit <id> --name NAME", resetText, "Change a habit's name.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "edit <id> --short SHORT", resetText, "Change a habit's short name.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "delete <id>", resetText, "Delete a habit (asks for confirmation).")
	
	// Data management
	fmt.Printf("\n%sData Management:%s\n", boldText, resetText)
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "export [--file FILE]", resetText, "Export habits data to a file.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "import --file FILE", resetText, "Import habits from a file.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "import --file FILE --merge", resetText, "Import and merge with existing habits.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "help", resetText, "Show this help message.")
	
	// Examples
	fmt.Printf("\n%sExamples:%s\n", boldText, resetText)
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "habits add \"Morning Exercise\"", resetText, "Add a new habit to track.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "habits done 1", resetText, "Mark habit #1 as done for today.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "habits tracker 2 -r month", resetText, "View month tracker for habit #2.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "habits stats", resetText, "Show statistics for all habits.")
	fmt.Printf("  %s%-*s%s %s\n", accentText, cmdWidth, "habits export -f backup.json", resetText, "Export your habit data.")
}

// showTrackerWithoutClearing shows the tracker but doesn't clear the screen
// This is mainly for use with the stats command
func showTrackerWithoutClearing(args []string, df *DataFile) {
	// Define flag set for view command
	viewCmd := flag.NewFlagSet("view", flag.ExitOnError)
	rangeFlag := viewCmd.String("range", "last30", "View range: year, month, week, day, last30")
	// Add short form flag as an alias
	rShortFlag := viewCmd.String("r", "", "Short form for --range")
	
	// Parse flags
	var flagArgs []string
	if len(args) > 1 {
		flagArgs = args[1:]
	} else {
		flagArgs = []string{}
	}
	
	err := viewCmd.Parse(flagArgs)
	if err != nil {
		return
	}
	
	// Get range value (prefer long form, fallback to short form)
	viewRange := *rangeFlag
	if viewRange == "last30" && *rShortFlag != "" {
		viewRange = *rShortFlag
	}
	
	// Find the habit
	habit, _ := findHabit(df, args[0])
	if habit == nil {
		fmt.Printf("Error: No habit found matching '%s'.\n", args[0])
		return
	}
	
	// Title without clearing screen
	fmt.Printf("\n📊 %sTracker: %s%s\n\n", boldText, habit.Name, resetText)
	
	// If day view, show the daily summary instead of grid
	if viewRange == "day" {
		showDayView(df, habit)
		return
	}

	completedDates := make(map[string]bool)
	for _, d := range habit.DatesTracked {
		completedDates[d] = true
	}
	
	// Determine time range based on viewRange
	var numWeeks int
	var startDate time.Time
	
	switch viewRange {
	case "year":
		numWeeks = 52
		startDate = calculateStartDate()
	case "month":
		numWeeks = 5 // Enough weeks to show a month
		startDate = calculateMonthStartDate()
	case "week":
		numWeeks = 1
		startDate = calculateWeekStartDate()
	case "last30":
		numWeeks = 5 // 5 weeks to ensure 30 days
		startDate = calculateLast30DaysStartDate()
	}
	
	// Generate grid data for a single habit
	gridData := make([]GridDay, 0, numWeeks*7)
	currentDate := startDate
	
	// Create a flat list of GridDay entries for the selected time period
	for i := 0; i < numWeeks*7; i++ {
		dateStr := currentDate.Format("2006-01-02")
		day := GridDay{
			Date:     currentDate,
			Done:     completedDates[dateStr],
			InFuture: currentDate.After(time.Now()),
		}
		gridData = append(gridData, day)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	printGrid(gridData, ViewSingleHabit, getTerminalWidth(), habit.Name)
}

func main() {
	df, err := loadData()
	if err != nil {
		// loadData now returns a more specific error
		fmt.Printf("Error loading data file (%s): %v\n", dataFilePath, err)
		// Attempt to provide more guidance
		if os.IsNotExist(err) {
			fmt.Println("The data file doesn't exist yet. It will be created when you add your first habit.")
		} else {
			fmt.Println("There might be an issue with the file format or permissions.")
		}
	}

	// Always check reminders unless it's the list command or no command or help
	runReminders := true
	if len(os.Args) < 2 || (len(os.Args) >= 2 && (os.Args[1] == "list" || os.Args[1] == "help")) {
		runReminders = false
	}
	// Don't run reminders if there are no habits yet.
	if len(df.Habits) == 0 {
		runReminders = false
	}
	// Don't run reminders if explicitly using the undone command (which shows reminders itself)
	if len(os.Args) >= 2 && os.Args[1] == "undone" {
		runReminders = false
	}
	// Only show reminders when running the base command with no arguments
	if len(os.Args) >= 2 {
		runReminders = false
	}

	if runReminders {
		needsReminder := checkReminders(df)
		printReminders(needsReminder)
	}

	if len(os.Args) < 2 {
		// Check if file exists, create if not (and possible)
		if _, err := os.Stat(dataFilePath); os.IsNotExist(err) {
			fmt.Println("No data file found. Creating an empty one.")
			saveData(&DataFile{Habits: []Habit{}}) // Save empty data to create the file
			return
		}
		
		// Show tracker with last 30 days view instead of just help
		commandViewAggregate(df, "last30")
		fmt.Println()
		fmt.Println("Use 'habits help' for more information.")
		return
	}

	subcommand := strings.ToLower(os.Args[1])
	args := os.Args[2:]

	switch subcommand {
	case "add":
		commandAdd(args, df)
	case "list":
		commandList(df)
	case "done":
		commandDone(args, df)
	case "remove":
		commandRemove(args, df)
	case "undone":
		commandUndone(df)
	case "tracker":
		// Define flag set for tracker command
		trackerCmd := flag.NewFlagSet("tracker", flag.ExitOnError)
		rangeFlag := trackerCmd.String("range", "last30", "View range: year, month, week, day, last30")
		// Add short form flag as an alias
		rShortFlag := trackerCmd.String("r", "", "Short form for --range")
		
		// Set usage message
		trackerCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: %s tracker [<id>] [--range <range>] or [-r <range>]\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Range options: year, month, week, day, last30\n")
			trackerCmd.PrintDefaults()
		}
		
		// Find the habit identifier and remaining flags
		var identifier string
		var flagArgs []string
		
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			identifier = args[0]
			flagArgs = args[1:]
		} else {
			flagArgs = args
		}
		
		// Parse flags
		err := trackerCmd.Parse(flagArgs)
		if err != nil {
			return
		}
		
		// Get range value (prefer long form, fallback to short form)
		viewRange := *rangeFlag
		if viewRange == "last30" && *rShortFlag != "" {
			viewRange = *rShortFlag
		}
		
		// Validate range
		if viewRange != "year" && viewRange != "month" && viewRange != "week" && viewRange != "day" && viewRange != "last30" {
			fmt.Printf("Error: Invalid range '%s'. Use year, month, week, day, or last30.\n", viewRange)
			return
		}
		
		// Process based on identifier and range
		if identifier == "" {
			// Aggregate view with range
			commandViewAggregate(df, viewRange)
		} else {
			// Single habit view with range
			commandView([]string{identifier, "--range", viewRange}, df)
		}
	case "stats":
		commandStats(args, df)
	case "edit":
		commandEdit(args, df)
	case "export":
		commandExport(args, df)
	case "import":
		commandImport(args, df)
	case "delete":
		commandDelete(args, df)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Printf("Error: Unknown subcommand '%s'.\n\n", subcommand)
		printHelp()
		os.Exit(1)
	}
}
