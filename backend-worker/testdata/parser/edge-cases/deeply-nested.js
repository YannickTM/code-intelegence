// Deeply nested code to test DEEP_NESTING diagnostic detection.
// Contains 12+ levels of nesting to exceed the threshold of 6.

function processData(input) {
  if (input) {
    for (let i = 0; i < input.length; i++) {
      if (input[i].active) {
        try {
          for (const key of Object.keys(input[i].data)) {
            if (key.startsWith("item")) {
              switch (input[i].data[key].type) {
                case "nested":
                  if (input[i].data[key].children) {
                    for (const child of input[i].data[key].children) {
                      if (child.valid) {
                        while (child.pending) {
                          if (child.retries > 0) {
                            try {
                              // Level 12+ nesting
                              console.log("deeply nested operation");
                              child.pending = false;
                            } catch (innerErr) {
                              child.retries--;
                            }
                          } else {
                            break;
                          }
                        }
                      }
                    }
                  }
                  break;
                case "flat":
                  console.log("flat item");
                  break;
              }
            }
          }
        } catch (err) {
          console.error("Error processing item", err);
        }
      }
    }
  }
}

module.exports = { processData };
