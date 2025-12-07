const path = require("path");

module.exports = (env = {}, argv = {}) => {
    const mode = argv.mode || env.mode || "production";

    return {
        mode,
        entry: {
            global: "./public/static/src/js/global.js",
            dashboard: "./public/static/src/js/dashboard.js",
            scoreboard: "./public/static/src/js/scoreboard.js"
        },
        output: {
            filename: "[name].js",
            path: path.resolve(__dirname, "public/static/build"),
            clean: {
                keep(asset) {
                    return asset.endsWith("app.css") || asset.endsWith("app.css.map");
                }
            }
        },
        resolve: {
            extensions: [".js"]
        },
        optimization: {
            minimize: true
        }
    };
};
