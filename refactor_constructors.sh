#!/bin/bash

# Update simple NewHandState calls with uniform chips in tests
find . -name "*_test.go" -type f -exec sed -i '' \
  's/NewHandState(\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^)]*\))/NewHand(rand.New(rand.NewSource(42)), \1, \2, \3, \4, WithUniformChips(\5))/g' {} \;

# Update NewHandStateWithChips calls
find . -name "*_test.go" -type f -exec sed -i '' \
  's/NewHandStateWithChips(\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^)]*\))/NewHand(rand.New(rand.NewSource(42)), \1, \3, \4, \5, WithChips(\2))/g' {} \;

# Update NewHandStateWithChipsAndRNG calls
find . -name "*_test.go" -type f -exec sed -i '' \
  's/NewHandStateWithChipsAndRNG(\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^)]*\))/NewHand(\6, \1, \3, \4, \5, WithChips(\2))/g' {} \;

# Update NewHandStateWithRNG calls
find . -name "*_test.go" -type f -exec sed -i '' \
  's/NewHandStateWithRNG(\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), \([^)]*\))/NewHand(\6, \1, \2, \3, \4, WithUniformChips(\5))/g' {} \;

echo "Replacements done. Manual review needed for complex cases."