const path = require("path");
const TerserPlugin = require("terser-webpack-plugin");

module.exports = (env = {}, argv = {}) => {
    const mode = argv.mode || env.mode || "production";

    return {
        mode: mode,
        devtool: mode === "development" ? "inline-source-map" : false,
        entry: {
            global: "./public/src/js/global.js",
            dashboard: "./public/src/js/dashboard.js",
            scoreboard: "./public/src/js/scoreboard.js"
        },
        output: {
            filename: "[name].js",
            path: path.resolve(__dirname, "public/build")
        },
        resolve: {
            extensions: [".js"]
        },
        optimization: {
            minimizer: [
                new TerserPlugin({
                    terserOptions: {
                        format: {
                            comments: false
                        }
                    },
                    extractComments: false
                })
            ],
            splitChunks: {
                chunks: "all"
            }
        }
    };
};
