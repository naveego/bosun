// Load tempDirectory before it gets wiped by tool-cache
let tempDirectory = process.env["RUNNER_TEMPDIRECTORY"] || "";
import * as os from "os";
import * as path from "path";

import * as core from "@actions/core";
import * as tc from "@actions/tool-cache";
import * as request from "request-promise-native";

let osPlat: string = os.platform();
let osArch: string = os.arch();

if (!tempDirectory) {
  let baseLocation;
  if (process.platform === "win32") {
    // On windows use the USERPROFILE env variable
    baseLocation = process.env["USERPROFILE"] || "C:\\";
  } else {
    if (process.platform === "darwin") {
      baseLocation = "/Users";
    } else {
      baseLocation = "/home";
    }
  }
  tempDirectory = path.join(baseLocation, "actions", "temp");
}

export async function downloadBosun(): Promise<string> {
  let latestUrl = "https://github.com/naveego/bosun/releases/latest";
  let latestResult = await request
    .get(latestUrl, { followRedirect: false, resolveWithFullResponse: true })
    .then(() => {
      throw new Error("Expected a redirect.");
    })
    .catch(error => {
      return error.response.headers.location;
    });

  console.log(latestResult);

  const parts = latestResult.split("/");
  const tag = parts[parts.length - 1];

  let toolPath = tc.find("bosun", tag);
  if (toolPath) {
    console.log("Using cached version " + tag);
  } else {
    console.log("Downloading bosun version ${tag}...");
    let packageUrl = latestUrl + getBosunFileName(tag);
    let downloadPath: string | null = null;

    try {
      downloadPath = await tc.downloadTool(packageUrl);
    } catch (error) {
      core.debug(error);

      throw `Failed to download ${packageUrl}: ${error}`;
    }

    //
    // Extract
    //
    let extPath: string = tempDirectory;
    if (!extPath) {
      throw new Error("Temp directory not set");
    }

    // console.log(downloadPath);
    extPath = await tc.extractTar(downloadPath);

    toolPath = await tc.cacheDir(extPath, "bosun", tag);
  }

  core.addPath(toolPath);

  return toolPath;
}

function getBosunFileName(tag: string): string {
  const fileArch = osArch === "x64" ? "amd64" : "386";
  let fileOS = osPlat === "win32" ? "windows" : osPlat;

  return `/download/bosun_${tag}_${fileOS}_${fileArch}.tar.gz`;
}
