import java.util.HashMap;
import java.util.Map;
import java.util.Scanner;

public class Main {
    public static void main(String[] args) {
        Scanner scanner = new Scanner(System.in);
        
        String text = scanner.nextLine();
        String[] words = text.split("\\s+");
        
        Map<String, Integer> wordCount = new HashMap<>();
        
        for (String word : words) {
            word = word.toLowerCase();
            if (!word.isEmpty()) {
                wordCount.put(word, wordCount.getOrDefault(word, 0) + 1);
            }
        }
        
        // sort the map by value desc if same value sort by key asc
        // print the sorted map
        wordCount.entrySet().stream()
            .sorted((entry1, entry2) -> {
                int cmp = entry2.getValue().compareTo(entry1.getValue());
                if (cmp == 0) {
                    return entry1.getKey().compareTo(entry2.getKey());
                }
                return cmp;
            })
            .forEach(entry -> System.out.println(entry.getKey() + ": " + entry.getValue()));
        
         // close the scanner
        scanner.close();
    }
}
