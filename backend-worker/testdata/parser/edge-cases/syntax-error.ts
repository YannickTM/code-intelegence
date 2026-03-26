// This file has intentional syntax errors for testing parser error handling.

export interface User {
  id: string;
  name: string
  // Missing closing brace

export function processUser(user: User) {
  const result = {
    name: user.name,
    // Unclosed object literal and function

const x: number = "not a number;  // unclosed string literal

class BrokenClass extends {  // missing parent class
  method( {  // unclosed parameter list
    return
  }
}

function orphanedBlock() {
  if (true) {
    console.log("ok")
  // missing closing braces for if and function
