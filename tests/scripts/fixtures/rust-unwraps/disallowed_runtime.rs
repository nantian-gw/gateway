#![forbid(unsafe_code)]

pub fn parse_number(input: &str) -> u64 {
    input.parse::<u64>().unwrap()
}
