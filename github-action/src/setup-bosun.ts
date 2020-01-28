import * as installer from "./installer";
import * as core from "@actions/core";
import * as path from "path";
import * as glob from "@actions/glob";

async function run() {
  try {
    console.log("Running installer in " + __dirname);

    const bosunPath = await installer.downloadBosun();

    console.log(`Downloaded Bosun: ${bosunPath}`);

    core.exportVariable(
      "BOSUN_CONFIG",
      path.join(__dirname, "/bosun/bosun.yaml")
    );
  } catch (error) {
    core.setFailed(error.message);
  }
}

run();
