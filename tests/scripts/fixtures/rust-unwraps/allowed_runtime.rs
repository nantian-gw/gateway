#![forbid(unsafe_code)]

pub fn parse_number(input: &str) -> Result<u64, std::num::ParseIntError> {
    input.parse()
}
