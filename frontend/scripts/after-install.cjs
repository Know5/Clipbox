const fs = require("node:fs");
const path = require("node:path");

// Some lint dependencies ship Go examples. Keep Go's ./... traversal inside
// our own module, without excluding frontend assets from the application.
const modules = path.resolve(__dirname, "..", "node_modules");
if (fs.existsSync(modules)) {
  fs.writeFileSync(path.join(modules, "go.mod"), "module clipbox/frontend-dependencies\n\ngo 1.26\n");
}
